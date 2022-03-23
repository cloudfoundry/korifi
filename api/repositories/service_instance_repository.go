package repositories

import (
	"context"
	"fmt"
	"sort"

	"code.cloudfoundry.org/cf-k8s-controllers/api/apierrors"
	"code.cloudfoundry.org/cf-k8s-controllers/api/authorization"
	servicesv1alpha1 "code.cloudfoundry.org/cf-k8s-controllers/controllers/apis/services/v1alpha1"
	"code.cloudfoundry.org/cf-k8s-controllers/controllers/webhooks"

	"github.com/google/uuid"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

//+kubebuilder:rbac:groups=services.cloudfoundry.org,resources=cfserviceinstances,verbs=list;create;get;delete

const (
	CFServiceInstanceGUIDLabel     = "services.cloudfoundry.org/service-instance-guid"
	ServiceInstanceResourceType    = "Service Instance"
	serviceBindingSecretTypePrefix = "servicebinding.io/"
)

type NamespaceGetter interface {
	GetNamespaceForServiceInstance(ctx context.Context, guid string) (string, error)
}

type ServiceInstanceRepo struct {
	namespaceRetriever   NamespaceRetriever
	userClientFactory    UserK8sClientFactory
	namespacePermissions *authorization.NamespacePermissions
}

func NewServiceInstanceRepo(
	namespaceRetriever NamespaceRetriever,
	userClientFactory UserK8sClientFactory,
	namespacePermissions *authorization.NamespacePermissions,
) *ServiceInstanceRepo {
	return &ServiceInstanceRepo{
		namespaceRetriever:   namespaceRetriever,
		userClientFactory:    userClientFactory,
		namespacePermissions: namespacePermissions,
	}
}

type CreateServiceInstanceMessage struct {
	Name        string
	SpaceGUID   string
	Credentials map[string]string
	Type        string
	Tags        []string
	Labels      map[string]string
	Annotations map[string]string
}

type ListServiceInstanceMessage struct {
	Names           []string
	SpaceGuids      []string
	OrderBy         string
	DescendingOrder bool
}

type DeleteServiceInstanceMessage struct {
	GUID      string
	SpaceGUID string
}

type ServiceInstanceRecord struct {
	Name       string
	GUID       string
	SpaceGUID  string
	SecretName string
	Tags       []string
	Type       string
	CreatedAt  string
	UpdatedAt  string
}

func (r *ServiceInstanceRepo) CreateServiceInstance(ctx context.Context, authInfo authorization.Info, message CreateServiceInstanceMessage) (ServiceInstanceRecord, error) {
	userClient, err := r.userClientFactory.BuildClient(authInfo)
	if err != nil {
		// untested
		return ServiceInstanceRecord{}, fmt.Errorf("failed to build user client: %w", err)
	}

	cfServiceInstance := message.toCFServiceInstance()
	err = userClient.Create(ctx, &cfServiceInstance)
	if err != nil {
		if webhooks.HasErrorCode(err, webhooks.DuplicateServiceInstanceNameError) {
			errorDetail := fmt.Sprintf("The service instance name is taken: %s.", message.Name)
			return ServiceInstanceRecord{}, apierrors.NewUnprocessableEntityError(err, errorDetail)
		}
		return ServiceInstanceRecord{}, apierrors.FromK8sError(err, ServiceInstanceResourceType)
	}

	secretObj := cfServiceInstanceToSecret(cfServiceInstance)
	_, err = controllerutil.CreateOrPatch(ctx, userClient, &secretObj, func() error {
		secretObj.StringData = message.Credentials
		if secretObj.StringData == nil {
			secretObj.StringData = map[string]string{}
		}
		updateSecretTypeFields(&secretObj)

		return nil
	})
	if err != nil {
		return ServiceInstanceRecord{}, apierrors.FromK8sError(err, ServiceInstanceResourceType)
	}

	return cfServiceInstanceToServiceInstanceRecord(cfServiceInstance), nil
}

func (r *ServiceInstanceRepo) ListServiceInstances(ctx context.Context, authInfo authorization.Info, message ListServiceInstanceMessage) ([]ServiceInstanceRecord, error) {
	nsList, err := r.namespacePermissions.GetAuthorizedSpaceNamespaces(ctx, authInfo)
	if err != nil {
		// untested
		return nil, fmt.Errorf("failed to list namespaces for spaces with user role bindings: %w", err)
	}

	userClient, err := r.userClientFactory.BuildClient(authInfo)
	if err != nil {
		return []ServiceInstanceRecord{}, fmt.Errorf("failed to build user client: %w", err)
	}

	var filteredServiceInstances []servicesv1alpha1.CFServiceInstance
	for ns := range nsList {
		serviceInstanceList := new(servicesv1alpha1.CFServiceInstanceList)
		err = userClient.List(ctx, serviceInstanceList, client.InNamespace(ns))
		if k8serrors.IsForbidden(err) {
			continue
		}
		if err != nil {
			return []ServiceInstanceRecord{}, fmt.Errorf("failed to list service instances in namespace %s: %w",
				ns,
				apierrors.FromK8sError(err, ServiceInstanceResourceType),
			)
		}
		filteredServiceInstances = append(filteredServiceInstances, applyServiceInstanceListFilter(serviceInstanceList.Items, message)...)
	}

	orderedServiceInstances := orderServiceInstances(filteredServiceInstances, message.OrderBy, message.DescendingOrder)

	return returnServiceInstanceList(orderedServiceInstances), nil
}

func (r *ServiceInstanceRepo) GetServiceInstance(ctx context.Context, authInfo authorization.Info, guid string) (ServiceInstanceRecord, error) {
	userClient, err := r.userClientFactory.BuildClient(authInfo)
	if err != nil {
		return ServiceInstanceRecord{}, fmt.Errorf("failed to build user client: %w", err)
	}

	namespace, err := r.namespaceRetriever.NamespaceFor(ctx, guid, ServiceInstanceResourceType)
	if err != nil {
		return ServiceInstanceRecord{}, fmt.Errorf("failed to get namespace for service instance: %w", err)
	}

	var serviceInstance servicesv1alpha1.CFServiceInstance
	if err := userClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: guid}, &serviceInstance); err != nil {
		return ServiceInstanceRecord{}, fmt.Errorf("failed to get service instance: %w", apierrors.FromK8sError(err, ServiceInstanceResourceType))
	}

	return cfServiceInstanceToServiceInstanceRecord(serviceInstance), nil
}

func (r *ServiceInstanceRepo) DeleteServiceInstance(ctx context.Context, authInfo authorization.Info, message DeleteServiceInstanceMessage) error {
	userClient, err := r.userClientFactory.BuildClient(authInfo)
	if err != nil {
		return fmt.Errorf("failed to build user client: %w", err)
	}

	serviceInstance := &servicesv1alpha1.CFServiceInstance{
		ObjectMeta: metav1.ObjectMeta{
			Name:      message.GUID,
			Namespace: message.SpaceGUID,
		},
	}

	if err := userClient.Delete(ctx, serviceInstance); err != nil {
		return fmt.Errorf("failed to delete service instance: %w", apierrors.FromK8sError(err, ServiceInstanceResourceType))
	}

	return nil
}

func (m CreateServiceInstanceMessage) toCFServiceInstance() servicesv1alpha1.CFServiceInstance {
	guid := uuid.NewString()
	return servicesv1alpha1.CFServiceInstance{
		ObjectMeta: metav1.ObjectMeta{
			Name:        guid,
			Namespace:   m.SpaceGUID,
			Labels:      m.Labels,
			Annotations: m.Annotations,
		},
		Spec: servicesv1alpha1.CFServiceInstanceSpec{
			Name:       m.Name,
			SecretName: guid,
			Type:       servicesv1alpha1.InstanceType(m.Type),
			Tags:       m.Tags,
		},
	}
}

func cfServiceInstanceToServiceInstanceRecord(cfServiceInstance servicesv1alpha1.CFServiceInstance) ServiceInstanceRecord {
	updatedAtTime, _ := getTimeLastUpdatedTimestamp(&cfServiceInstance.ObjectMeta)

	return ServiceInstanceRecord{
		Name:       cfServiceInstance.Spec.Name,
		GUID:       cfServiceInstance.Name,
		SpaceGUID:  cfServiceInstance.Namespace,
		SecretName: cfServiceInstance.Spec.SecretName,
		Tags:       cfServiceInstance.Spec.Tags,
		Type:       string(cfServiceInstance.Spec.Type),
		CreatedAt:  cfServiceInstance.CreationTimestamp.UTC().Format(TimestampFormat),
		UpdatedAt:  updatedAtTime,
	}
}

func cfServiceInstanceToSecret(cfServiceInstance servicesv1alpha1.CFServiceInstance) corev1.Secret {
	labels := make(map[string]string, 1)
	labels[CFServiceInstanceGUIDLabel] = cfServiceInstance.Name

	return corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cfServiceInstance.Name,
			Namespace: cfServiceInstance.Namespace,
			Labels:    labels,
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: servicesv1alpha1.GroupVersion.String(),
					Kind:       "CFServiceInstance",
					Name:       cfServiceInstance.Name,
					UID:        cfServiceInstance.UID,
				},
			},
		},
	}
}

func applyServiceInstanceListFilter(serviceInstanceList []servicesv1alpha1.CFServiceInstance, message ListServiceInstanceMessage) []servicesv1alpha1.CFServiceInstance {
	if len(message.Names) == 0 && len(message.SpaceGuids) == 0 {
		return serviceInstanceList
	}

	var filtered []servicesv1alpha1.CFServiceInstance
	for _, serviceInstance := range serviceInstanceList {
		if matchesFilter(serviceInstance.Spec.Name, message.Names) &&
			matchesFilter(serviceInstance.Namespace, message.SpaceGuids) {
			filtered = append(filtered, serviceInstance)
		}
	}

	return filtered
}

func returnServiceInstanceList(serviceInstanceList []servicesv1alpha1.CFServiceInstance) []ServiceInstanceRecord {
	serviceInstanceRecords := make([]ServiceInstanceRecord, 0, len(serviceInstanceList))

	for _, serviceInstance := range serviceInstanceList {
		serviceInstanceRecords = append(serviceInstanceRecords, cfServiceInstanceToServiceInstanceRecord(serviceInstance))
	}
	return serviceInstanceRecords
}

func orderServiceInstances(serviceInstances []servicesv1alpha1.CFServiceInstance, sortBy string, desc bool) []servicesv1alpha1.CFServiceInstance {
	sort.Slice(serviceInstances, func(i, j int) bool {
		var less bool

		switch sortBy {
		case "created_at":
			less = serviceInstances[i].CreationTimestamp.Before(&serviceInstances[j].CreationTimestamp)
		case "updated_at":
			// Ignoring the errors that could be returned as there is no way to handle them
			updateTime1, _ := getTimeLastUpdatedTimestamp(&serviceInstances[i].ObjectMeta)
			updateTime2, _ := getTimeLastUpdatedTimestamp(&serviceInstances[j].ObjectMeta)
			less = updateTime1 < updateTime2
		default:
			// Default to sorting by name
			less = serviceInstances[i].Spec.Name < serviceInstances[j].Spec.Name
		}

		if desc {
			return !less
		}

		return less
	})

	return serviceInstances
}

func updateSecretTypeFields(secret *corev1.Secret) {
	userSpecifiedType, typeSpecified := secret.StringData["type"]
	if typeSpecified {
		secret.Type = corev1.SecretType(serviceBindingSecretTypePrefix + userSpecifiedType)
	} else {
		secret.StringData["type"] = servicesv1alpha1.UserProvidedType
		secret.Type = serviceBindingSecretTypePrefix + servicesv1alpha1.UserProvidedType
	}
}
