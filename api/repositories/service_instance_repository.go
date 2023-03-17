package repositories

import (
	"context"
	"fmt"

	"code.cloudfoundry.org/korifi/api/authorization"
	apierrors "code.cloudfoundry.org/korifi/api/errors"
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/tools/k8s"

	"github.com/google/uuid"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	CFServiceInstanceGUIDLabel     = "korifi.cloudfoundry.org/service-instance-guid"
	ServiceInstanceResourceType    = "Service Instance"
	serviceBindingSecretTypePrefix = "servicebinding.io/"
)

type NamespaceGetter interface {
	GetNamespaceForServiceInstance(ctx context.Context, guid string) (string, error)
}

type ServiceInstanceRepo struct {
	namespaceRetriever   NamespaceRetriever
	userClientFactory    authorization.UserK8sClientFactory
	namespacePermissions *authorization.NamespacePermissions
}

func NewServiceInstanceRepo(
	namespaceRetriever NamespaceRetriever,
	userClientFactory authorization.UserK8sClientFactory,
	namespacePermissions *authorization.NamespacePermissions,
) *ServiceInstanceRepo {
	return &ServiceInstanceRepo{
		namespaceRetriever:   namespaceRetriever,
		userClientFactory:    userClientFactory,
		namespacePermissions: namespacePermissions,
	}
}

type CreateServiceInstanceMessage struct {
	Name            string
	SpaceGUID       string
	ServicePlanGUID string
	Credentials     map[string]string
	Type            string
	Tags            []string
	Labels          map[string]string
	Annotations     map[string]string
}

type PatchServiceInstanceMessage struct {
	GUID        string
	SpaceGUID   string
	Name        *string
	Credentials *map[string]string
	Tags        *[]string
	MetadataPatch
}

func (p PatchServiceInstanceMessage) Apply(cfServiceInstance *korifiv1alpha1.CFServiceInstance) {
	if p.Name != nil {
		cfServiceInstance.Spec.DisplayName = *p.Name
	}
	if p.Tags != nil {
		cfServiceInstance.Spec.Tags = *p.Tags
	}
	p.MetadataPatch.Apply(cfServiceInstance)
}

type ListServiceInstanceMessage struct {
	Names          []string
	SpaceGuids     []string
	LabelSelectors []string
}

type DeleteServiceInstanceMessage struct {
	GUID      string
	SpaceGUID string
}

type ServiceInstanceRecord struct {
	Name        string
	GUID        string
	SpaceGUID   string
	SecretName  string
	Tags        []string
	Type        string
	Labels      map[string]string
	Annotations map[string]string
	CreatedAt   string
	UpdatedAt   string
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
		return ServiceInstanceRecord{}, apierrors.FromK8sError(err, ServiceInstanceResourceType)
	}

	if message.Type == korifiv1alpha1.UserProvidedType {
		secretObj := cfServiceInstanceToSecret(cfServiceInstance)
		_, err = controllerutil.CreateOrPatch(ctx, userClient, &secretObj, func() error {
			secretObj.StringData = message.Credentials
			if secretObj.StringData == nil {
				secretObj.StringData = map[string]string{}
			}
			createSecretTypeFields(&secretObj)

			return nil
		})
		if err != nil {
			return ServiceInstanceRecord{}, apierrors.FromK8sError(err, ServiceInstanceResourceType)
		}
	}

	return cfServiceInstanceToServiceInstanceRecord(cfServiceInstance), nil
}

func (r *ServiceInstanceRepo) PatchServiceInstance(ctx context.Context, authInfo authorization.Info, message PatchServiceInstanceMessage) (ServiceInstanceRecord, error) {
	userClient, err := r.userClientFactory.BuildClient(authInfo)
	if err != nil {
		return ServiceInstanceRecord{}, fmt.Errorf("failed to build user client: %w", err)
	}

	var cfServiceInstance korifiv1alpha1.CFServiceInstance
	cfServiceInstance.Namespace = message.SpaceGUID
	cfServiceInstance.Name = message.GUID
	if err = userClient.Get(ctx, client.ObjectKeyFromObject(&cfServiceInstance), &cfServiceInstance); err != nil {
		return ServiceInstanceRecord{}, apierrors.FromK8sError(err, ServiceInstanceResourceType)
	}

	err = k8s.PatchResource(ctx, userClient, &cfServiceInstance, func() {
		message.Apply(&cfServiceInstance)
	})
	if err != nil {
		return ServiceInstanceRecord{}, apierrors.FromK8sError(err, ServiceInstanceResourceType)
	}

	if message.Credentials != nil {
		secretObj := new(corev1.Secret)
		if err = userClient.Get(ctx, client.ObjectKey{Name: cfServiceInstance.Spec.SecretName, Namespace: cfServiceInstance.Namespace}, secretObj); err != nil {
			return ServiceInstanceRecord{}, err
		}

		if _, ok := (*message.Credentials)["type"]; !ok {
			(*message.Credentials)["type"] = string(secretObj.Data["type"])
		}

		newType := (*message.Credentials)["type"]
		if string(secretObj.Data["type"]) != (*message.Credentials)["type"] {
			return ServiceInstanceRecord{}, apierrors.NewInvalidRequestError(
				fmt.Errorf("cannot modify credential type: currently '%s': updating to '%s'", string(secretObj.Data["type"]), newType),
				"Cannot change credential type. Consider creating a new Service Instance.",
			)
		}

		_, err = controllerutil.CreateOrPatch(ctx, userClient, secretObj, func() error {
			data := map[string][]byte{}
			for k, v := range *message.Credentials {
				data[k] = []byte(v)
			}
			secretObj.Data = data

			return nil
		})
		if err != nil {
			return ServiceInstanceRecord{}, apierrors.FromK8sError(err, ServiceInstanceResourceType)
		}
	}

	return cfServiceInstanceToServiceInstanceRecord(cfServiceInstance), nil
}

// nolint:dupl
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

	var filteredServiceInstances []korifiv1alpha1.CFServiceInstance
	for ns := range nsList {
		serviceInstanceList := new(korifiv1alpha1.CFServiceInstanceList)
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

	return returnServiceInstanceList(filteredServiceInstances), nil
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

	var serviceInstance korifiv1alpha1.CFServiceInstance
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

	serviceInstance := &korifiv1alpha1.CFServiceInstance{
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

func (m CreateServiceInstanceMessage) toCFServiceInstance() korifiv1alpha1.CFServiceInstance {
	guid := uuid.NewString()
	return korifiv1alpha1.CFServiceInstance{
		ObjectMeta: metav1.ObjectMeta{
			Name:        guid,
			Namespace:   m.SpaceGUID,
			Labels:      m.Labels,
			Annotations: m.Annotations,
		},
		Spec: korifiv1alpha1.CFServiceInstanceSpec{
			DisplayName:     m.Name,
			SecretName:      guid,
			Type:            korifiv1alpha1.InstanceType(m.Type),
			Tags:            m.Tags,
			ServicePlanGUID: m.ServicePlanGUID,
		},
	}
}

func cfServiceInstanceToServiceInstanceRecord(cfServiceInstance korifiv1alpha1.CFServiceInstance) ServiceInstanceRecord {
	updatedAtTime, _ := getTimeLastUpdatedTimestamp(&cfServiceInstance.ObjectMeta)

	return ServiceInstanceRecord{
		Name:        cfServiceInstance.Spec.DisplayName,
		GUID:        cfServiceInstance.Name,
		SpaceGUID:   cfServiceInstance.Namespace,
		SecretName:  cfServiceInstance.Spec.SecretName,
		Tags:        cfServiceInstance.Spec.Tags,
		Type:        string(cfServiceInstance.Spec.Type),
		Labels:      cfServiceInstance.Labels,
		Annotations: cfServiceInstance.Annotations,
		CreatedAt:   cfServiceInstance.CreationTimestamp.UTC().Format(TimestampFormat),
		UpdatedAt:   updatedAtTime,
	}
}

func cfServiceInstanceToSecret(cfServiceInstance korifiv1alpha1.CFServiceInstance) corev1.Secret {
	labels := make(map[string]string, 1)
	labels[CFServiceInstanceGUIDLabel] = cfServiceInstance.Name

	return corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cfServiceInstance.Name,
			Namespace: cfServiceInstance.Namespace,
			Labels:    labels,
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: korifiv1alpha1.GroupVersion.String(),
					Kind:       "CFServiceInstance",
					Name:       cfServiceInstance.Name,
					UID:        cfServiceInstance.UID,
				},
			},
		},
	}
}

func applyServiceInstanceListFilter(serviceInstanceList []korifiv1alpha1.CFServiceInstance, message ListServiceInstanceMessage) []korifiv1alpha1.CFServiceInstance {
	if len(message.Names) == 0 && len(message.SpaceGuids) == 0 {
		return serviceInstanceList
	}

	var filtered []korifiv1alpha1.CFServiceInstance
	for _, serviceInstance := range serviceInstanceList {
		if matchesFilter(serviceInstance.Spec.DisplayName, message.Names) &&
			matchesFilter(serviceInstance.Namespace, message.SpaceGuids) &&
			labelsFilters(serviceInstance.Labels, message.LabelSelectors) {
			filtered = append(filtered, serviceInstance)
		}
	}

	return filtered
}

func returnServiceInstanceList(serviceInstanceList []korifiv1alpha1.CFServiceInstance) []ServiceInstanceRecord {
	serviceInstanceRecords := make([]ServiceInstanceRecord, 0, len(serviceInstanceList))

	for _, serviceInstance := range serviceInstanceList {
		serviceInstanceRecords = append(serviceInstanceRecords, cfServiceInstanceToServiceInstanceRecord(serviceInstance))
	}
	return serviceInstanceRecords
}

func createSecretTypeFields(secret *corev1.Secret) {
	userSpecifiedType, typeSpecified := secret.StringData["type"]
	if typeSpecified {
		secret.Type = corev1.SecretType(serviceBindingSecretTypePrefix + userSpecifiedType)
	} else {
		secret.StringData["type"] = korifiv1alpha1.UserProvidedType
		secret.Type = serviceBindingSecretTypePrefix + korifiv1alpha1.UserProvidedType
	}
}
