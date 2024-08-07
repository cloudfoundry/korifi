package repositories

import (
	"context"
	"errors"
	"fmt"
	"time"

	"code.cloudfoundry.org/korifi/api/authorization"
	apierrors "code.cloudfoundry.org/korifi/api/errors"
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/tools"
	"code.cloudfoundry.org/korifi/tools/k8s"

	"github.com/BooleanCat/go-functional/iter"
	"github.com/google/uuid"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	CFServiceInstanceGUIDLabel  = "korifi.cloudfoundry.org/service-instance-guid"
	ServiceInstanceResourceType = "Service Instance"
)

type NamespaceGetter interface {
	GetNamespaceForServiceInstance(ctx context.Context, guid string) (string, error)
}

type ServiceInstanceRepo struct {
	namespaceRetriever   NamespaceRetriever
	userClientFactory    authorization.UserK8sClientFactory
	namespacePermissions *authorization.NamespacePermissions
	awaiter              Awaiter[*korifiv1alpha1.CFServiceInstance]
}

func NewServiceInstanceRepo(
	namespaceRetriever NamespaceRetriever,
	userClientFactory authorization.UserK8sClientFactory,
	namespacePermissions *authorization.NamespacePermissions,
	awaiter Awaiter[*korifiv1alpha1.CFServiceInstance],
) *ServiceInstanceRepo {
	return &ServiceInstanceRepo{
		namespaceRetriever:   namespaceRetriever,
		userClientFactory:    userClientFactory,
		namespacePermissions: namespacePermissions,
		awaiter:              awaiter,
	}
}

type CreateServiceInstanceMessage struct {
	Name        string
	SpaceGUID   string
	Credentials map[string]any
	Type        string
	Tags        []string
	Labels      map[string]string
	Annotations map[string]string
}

type PatchServiceInstanceMessage struct {
	GUID        string
	SpaceGUID   string
	Name        *string
	Credentials *map[string]any
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
	Names         []string
	SpaceGUIDs    []string
	GUIDs         []string
	LabelSelector string
}

func (m *ListServiceInstanceMessage) matches(serviceInstance korifiv1alpha1.CFServiceInstance) bool {
	return tools.EmptyOrContains(m.Names, serviceInstance.Spec.DisplayName) &&
		tools.EmptyOrContains(m.GUIDs, serviceInstance.Name)
}

func (m *ListServiceInstanceMessage) matchesNamespace(ns string) bool {
	return tools.EmptyOrContains(m.SpaceGUIDs, ns)
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
	CreatedAt   time.Time
	UpdatedAt   *time.Time
}

func (r *ServiceInstanceRepo) CreateServiceInstance(ctx context.Context, authInfo authorization.Info, message CreateServiceInstanceMessage) (ServiceInstanceRecord, error) {
	userClient, err := r.userClientFactory.BuildClient(authInfo)
	if err != nil {
		return ServiceInstanceRecord{}, fmt.Errorf("failed to build user client: %w", err)
	}

	cfServiceInstance := message.toCFServiceInstance()
	err = userClient.Create(ctx, cfServiceInstance)
	if err != nil {
		return ServiceInstanceRecord{}, apierrors.FromK8sError(err, ServiceInstanceResourceType)
	}

	err = r.createCredentialsSecret(ctx, userClient, cfServiceInstance, message.Credentials)
	if err != nil {
		return ServiceInstanceRecord{}, apierrors.FromK8sError(err, ServiceInstanceResourceType)
	}

	return cfServiceInstanceToRecord(*cfServiceInstance), nil
}

func (r *ServiceInstanceRepo) PatchServiceInstance(ctx context.Context, authInfo authorization.Info, message PatchServiceInstanceMessage) (ServiceInstanceRecord, error) {
	userClient, err := r.userClientFactory.BuildClient(authInfo)
	if err != nil {
		return ServiceInstanceRecord{}, fmt.Errorf("failed to build user client: %w", err)
	}

	cfServiceInstance := &korifiv1alpha1.CFServiceInstance{}
	cfServiceInstance.Namespace = message.SpaceGUID
	cfServiceInstance.Name = message.GUID
	if err = userClient.Get(ctx, client.ObjectKeyFromObject(cfServiceInstance), cfServiceInstance); err != nil {
		return ServiceInstanceRecord{}, apierrors.FromK8sError(err, ServiceInstanceResourceType)
	}

	err = k8s.PatchResource(ctx, userClient, cfServiceInstance, func() {
		message.Apply(cfServiceInstance)
	})
	if err != nil {
		return ServiceInstanceRecord{}, apierrors.FromK8sError(err, ServiceInstanceResourceType)
	}

	if message.Credentials != nil {
		cfServiceInstance, err = r.migrateLegacyCredentials(ctx, userClient, cfServiceInstance)
		if err != nil {
			return ServiceInstanceRecord{}, err
		}
		err = r.patchCredentialsSecret(ctx, userClient, cfServiceInstance, *message.Credentials)
		if err != nil {
			return ServiceInstanceRecord{}, apierrors.FromK8sError(err, ServiceInstanceResourceType)
		}
	}

	return cfServiceInstanceToRecord(*cfServiceInstance), nil
}

func (r *ServiceInstanceRepo) migrateLegacyCredentials(ctx context.Context, userClient client.WithWatch, cfServiceInstance *korifiv1alpha1.CFServiceInstance) (*korifiv1alpha1.CFServiceInstance, error) {
	cfServiceInstance, err := r.awaiter.AwaitCondition(ctx, userClient, cfServiceInstance, korifiv1alpha1.StatusConditionReady)
	if err != nil {
		return nil, err
	}
	err = k8s.PatchResource(ctx, userClient, cfServiceInstance, func() {
		cfServiceInstance.Spec.SecretName = cfServiceInstance.Status.Credentials.Name
	})
	if err != nil {
		return nil, apierrors.FromK8sError(err, ServiceInstanceResourceType)
	}

	return cfServiceInstance, nil
}

func (r *ServiceInstanceRepo) patchCredentialsSecret(
	ctx context.Context,
	userClient client.Client,
	cfServiceInstance *korifiv1alpha1.CFServiceInstance,
	credentials map[string]any,
) error {
	credentialsSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cfServiceInstance.Spec.SecretName,
			Namespace: cfServiceInstance.Namespace,
		},
	}

	credentialsSecretData, err := tools.ToCredentialsSecretData(credentials)
	if err != nil {
		return errors.New("failed to marshal credentials for service instance")
	}
	return PatchResource(ctx, userClient, credentialsSecret, func() {
		credentialsSecret.Data = credentialsSecretData
	})
}

func (r *ServiceInstanceRepo) createCredentialsSecret(
	ctx context.Context,
	userClient client.Client,
	cfServiceInstance *korifiv1alpha1.CFServiceInstance,
	creds map[string]any,
) error {
	credentialsSecretData, err := tools.ToCredentialsSecretData(creds)
	if err != nil {
		return errors.New("failed to marshal credentials for service instance")
	}

	credentialsSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cfServiceInstance.Spec.SecretName,
			Namespace: cfServiceInstance.Namespace,
			Labels: map[string]string{
				CFServiceInstanceGUIDLabel: cfServiceInstance.Name,
			},
		},
		Data: credentialsSecretData,
	}
	_ = controllerutil.SetOwnerReference(cfServiceInstance, credentialsSecret, scheme.Scheme)

	return userClient.Create(ctx, credentialsSecret)
}

// nolint:dupl
func (r *ServiceInstanceRepo) ListServiceInstances(ctx context.Context, authInfo authorization.Info, message ListServiceInstanceMessage) ([]ServiceInstanceRecord, error) {
	userClient, err := r.userClientFactory.BuildClient(authInfo)
	if err != nil {
		return []ServiceInstanceRecord{}, fmt.Errorf("failed to build user client: %w", err)
	}

	authorizedSpaceNamespacesIter, err := authorizedSpaceNamespaces(ctx, authInfo, r.namespacePermissions)
	if err != nil {
		return nil, fmt.Errorf("failed to list namespaces for spaces with user role bindings: %w", err)
	}

	labelSelector, err := labels.Parse(message.LabelSelector)
	if err != nil {
		return []ServiceInstanceRecord{}, apierrors.NewUnprocessableEntityError(err, "invalid label selector")
	}

	nsList := authorizedSpaceNamespacesIter.Filter(message.matchesNamespace).Collect()
	var serviceInstances []korifiv1alpha1.CFServiceInstance
	for _, ns := range nsList {
		serviceInstanceList := new(korifiv1alpha1.CFServiceInstanceList)
		err = userClient.List(ctx, serviceInstanceList, client.InNamespace(ns), &client.ListOptions{LabelSelector: labelSelector})
		if k8serrors.IsForbidden(err) {
			continue
		}
		if err != nil {
			return []ServiceInstanceRecord{}, fmt.Errorf("failed to list service instances in namespace %s: %w",
				ns,
				apierrors.FromK8sError(err, ServiceInstanceResourceType),
			)
		}
		serviceInstances = append(serviceInstances, serviceInstanceList.Items...)
	}

	filteredServiceInstances := iter.Lift(serviceInstances).Filter(message.matches)
	return iter.Map(filteredServiceInstances, cfServiceInstanceToRecord).Collect(), nil
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

	serviceInstance := &korifiv1alpha1.CFServiceInstance{}
	if err := userClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: guid}, serviceInstance); err != nil {
		return ServiceInstanceRecord{}, fmt.Errorf("failed to get service instance: %w", apierrors.FromK8sError(err, ServiceInstanceResourceType))
	}

	return cfServiceInstanceToRecord(*serviceInstance), nil
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

func (m CreateServiceInstanceMessage) toCFServiceInstance() *korifiv1alpha1.CFServiceInstance {
	guid := uuid.NewString()
	return &korifiv1alpha1.CFServiceInstance{
		ObjectMeta: metav1.ObjectMeta{
			Name:        guid,
			Namespace:   m.SpaceGUID,
			Labels:      m.Labels,
			Annotations: m.Annotations,
		},
		Spec: korifiv1alpha1.CFServiceInstanceSpec{
			DisplayName: m.Name,
			SecretName:  guid,
			Type:        korifiv1alpha1.InstanceType(m.Type),
			Tags:        m.Tags,
		},
	}
}

func cfServiceInstanceToRecord(cfServiceInstance korifiv1alpha1.CFServiceInstance) ServiceInstanceRecord {
	return ServiceInstanceRecord{
		Name:        cfServiceInstance.Spec.DisplayName,
		GUID:        cfServiceInstance.Name,
		SpaceGUID:   cfServiceInstance.Namespace,
		SecretName:  cfServiceInstance.Spec.SecretName,
		Tags:        cfServiceInstance.Spec.Tags,
		Type:        string(cfServiceInstance.Spec.Type),
		Labels:      cfServiceInstance.Labels,
		Annotations: cfServiceInstance.Annotations,
		CreatedAt:   cfServiceInstance.CreationTimestamp.Time,
		UpdatedAt:   getLastUpdatedTime(&cfServiceInstance),
	}
}
