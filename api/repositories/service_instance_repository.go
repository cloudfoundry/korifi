package repositories

import (
	"context"
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/client"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"

	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	corev1 "k8s.io/api/core/v1"

	servicesv1alpha1 "code.cloudfoundry.org/cf-k8s-controllers/controllers/apis/services/v1alpha1"
	"github.com/google/uuid"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"code.cloudfoundry.org/cf-k8s-controllers/api/authorization"
)

//+kubebuilder:rbac:groups=services.cloudfoundry.org,resources=cfserviceinstances,verbs=list;create

const (
	CFServiceInstanceGUIDLabel          = "services.cloudfoundry.org/service-instance-guid"
	ServiceInstanceCredentialSecretType = "servicebinding.io/user-provided"
)

type ServiceInstanceRepo struct {
	userClientFactory    UserK8sClientFactory
	namespacePermissions *authorization.NamespacePermissions
}

func NewServiceInstanceRepo(
	userClientFactory UserK8sClientFactory,
	namespacePermissions *authorization.NamespacePermissions,
) *ServiceInstanceRepo {
	return &ServiceInstanceRepo{
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
	Names      []string
	SpaceGuids []string
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
		if apierrors.IsForbidden(err) {
			return ServiceInstanceRecord{}, NewForbiddenError(err)
		}
		// untested
		return ServiceInstanceRecord{}, err
	}

	secretObj := cfServiceInstanceToSecret(cfServiceInstance)
	_, err = controllerutil.CreateOrPatch(ctx, userClient, &secretObj, func() error {
		secretObj.StringData = message.Credentials
		return nil
	})
	if err != nil {
		// untested
		return ServiceInstanceRecord{}, err
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
		// untested
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
			// untested
			return []ServiceInstanceRecord{}, fmt.Errorf("failed to list service instances in namespace %s: %w", ns, err)
		}
		filteredServiceInstances = append(filteredServiceInstances, applyServiceInstanceListFilter(serviceInstanceList.Items, message)...)
	}

	return returnServiceInstanceList(filteredServiceInstances), nil
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
		Type: ServiceInstanceCredentialSecretType,
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

func matchesFilter(field string, filter []string) bool {
	if len(filter) == 0 {
		return true
	}

	for _, value := range filter {
		if field == value {
			return true
		}
	}

	return false
}

func returnServiceInstanceList(serviceInstanceList []servicesv1alpha1.CFServiceInstance) []ServiceInstanceRecord {
	serviceInstanceRecords := make([]ServiceInstanceRecord, 0, len(serviceInstanceList))

	for _, serviceInstance := range serviceInstanceList {
		serviceInstanceRecords = append(serviceInstanceRecords, cfServiceInstanceToServiceInstanceRecord(serviceInstance))
	}
	return serviceInstanceRecords
}
