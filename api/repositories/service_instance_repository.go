package repositories

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"time"

	"code.cloudfoundry.org/korifi/api/authorization"
	apierrors "code.cloudfoundry.org/korifi/api/errors"
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/controllers/services"
	"code.cloudfoundry.org/korifi/tools"
	"code.cloudfoundry.org/korifi/tools/k8s"
	"golang.org/x/exp/maps"

	"github.com/google/uuid"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	CFServiceInstanceGUIDLabel          = "korifi.cloudfoundry.org/service-instance-guid"
	ServiceInstanceResourceType         = "Service Instance"
	CredentialsSecretAvailableCondition = "CredentialSecretAvailable"

	credentialsTypeKey = "type"
)

type NamespaceGetter interface {
	GetNamespaceForServiceInstance(ctx context.Context, guid string) (string, error)
}

type ServiceInstanceRepo struct {
	namespaceRetriever   NamespaceRetriever
	userClientFactory    authorization.UserK8sClientFactory
	namespacePermissions *authorization.NamespacePermissions
	awaiter              ConditionAwaiter[*korifiv1alpha1.CFServiceInstance]
}

func NewServiceInstanceRepo(
	namespaceRetriever NamespaceRetriever,
	userClientFactory authorization.UserK8sClientFactory,
	namespacePermissions *authorization.NamespacePermissions,
	awaiter ConditionAwaiter[*korifiv1alpha1.CFServiceInstance],
) *ServiceInstanceRepo {
	return &ServiceInstanceRepo{
		namespaceRetriever:   namespaceRetriever,
		userClientFactory:    userClientFactory,
		namespacePermissions: namespacePermissions,
		awaiter:              awaiter,
	}
}

type CreateServiceInstanceMessage struct {
	Name            string
	SpaceGUID       string
	ServicePlanGUID string
	Credentials     map[string]any
	Type            string
	Tags            []string
	Labels          map[string]string
	Annotations     map[string]string
	Parameters      map[string]any
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
	PlanGUID    string
	Labels      map[string]string
	Annotations map[string]string
	CreatedAt   time.Time
	UpdatedAt   *time.Time
	Parameters  map[string]any
	State       *korifiv1alpha1.CFResourceState
}

func (r *ServiceInstanceRepo) CreateServiceInstance(ctx context.Context, authInfo authorization.Info, message CreateServiceInstanceMessage) (ServiceInstanceRecord, error) {
	userClient, err := r.userClientFactory.BuildClient(authInfo)
	if err != nil {
		// untested
		return ServiceInstanceRecord{}, fmt.Errorf("failed to build user client: %w", err)
	}

	cfServiceInstance, err := message.toCFServiceInstance()
	if err != nil {
		return ServiceInstanceRecord{}, err
	}
	err = userClient.Create(ctx, cfServiceInstance)
	if err != nil {
		return ServiceInstanceRecord{}, apierrors.FromK8sError(err, ServiceInstanceResourceType)
	}

	if message.Type == korifiv1alpha1.UserProvidedType {
		err = r.createCredentialsSecret(ctx, userClient, cfServiceInstance, defaultCredentialsType(message.Credentials))
		if err != nil {
			return ServiceInstanceRecord{}, apierrors.FromK8sError(err, ServiceInstanceResourceType)
		}
	}

	return cfServiceInstanceToServiceInstanceRecord(cfServiceInstance)
}

func defaultCredentialsType(credentials map[string]any) map[string]any {
	result := map[string]any{}
	maps.Copy(result, credentials)
	if _, hasCredentialsType := result[credentialsTypeKey]; !hasCredentialsType {
		result[credentialsTypeKey] = korifiv1alpha1.UserProvidedType
	}

	return result
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

	return cfServiceInstanceToServiceInstanceRecord(cfServiceInstance)
}

func (r *ServiceInstanceRepo) migrateLegacyCredentials(ctx context.Context, userClient client.WithWatch, cfServiceInstance *korifiv1alpha1.CFServiceInstance) (*korifiv1alpha1.CFServiceInstance, error) {
	cfServiceInstance, err := r.awaiter.AwaitCondition(ctx, userClient, cfServiceInstance, CredentialsSecretAvailableCondition)
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
	newCredentials := map[string]any{}
	maps.Copy(newCredentials, credentials)

	credentialsSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cfServiceInstance.Spec.SecretName,
			Namespace: cfServiceInstance.Namespace,
		},
	}

	err := userClient.Get(ctx, client.ObjectKeyFromObject(credentialsSecret), credentialsSecret)
	if err != nil {
		return err
	}

	oldCredentials := fromSecretData(credentialsSecret.Data)
	err = validateCredentialsChange(oldCredentials, newCredentials)
	if err != nil {
		return err
	}

	if oldCredentialType, hasCredentialsType := oldCredentials[credentialsTypeKey]; hasCredentialsType {
		newCredentials[credentialsTypeKey] = oldCredentialType
	}

	return r.createCredentialsSecret(ctx, userClient, cfServiceInstance, newCredentials)
}

func (r *ServiceInstanceRepo) createCredentialsSecret(
	ctx context.Context,
	userClient client.Client,
	cfServiceInstance *korifiv1alpha1.CFServiceInstance,
	credentials map[string]any,
) error {
	newCredentials := map[string]any{}
	maps.Copy(newCredentials, credentials)

	credentialsSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cfServiceInstance.Spec.SecretName,
			Namespace: cfServiceInstance.Namespace,
		},
	}

	_, err := controllerutil.CreateOrPatch(ctx, userClient, credentialsSecret, func() error {
		if credentialsSecret.Labels == nil {
			credentialsSecret.Labels = map[string]string{}
		}
		credentialsSecret.Labels[CFServiceInstanceGUIDLabel] = cfServiceInstance.Name

		var err error
		credentialsSecret.Data, err = toSecretData(newCredentials)
		if err != nil {
			return errors.New("failed to marshal credentials for service instance")
		}

		return controllerutil.SetOwnerReference(cfServiceInstance, credentialsSecret, scheme.Scheme)
	})
	return err
}

func toSecretData(credentials map[string]any) (map[string][]byte, error) {
	var credentialBytes []byte
	credentialBytes, err := json.Marshal(credentials)
	if err != nil {
		return nil, errors.New("failed to marshal credentials for service instance")
	}

	return map[string][]byte{
		korifiv1alpha1.CredentialsSecretKey: credentialBytes,
	}, nil
}

func fromSecretData(credentialsSecretData map[string][]byte) map[string]any {
	credentialsBytes, hasCredentials := credentialsSecretData[korifiv1alpha1.CredentialsSecretKey]
	if !hasCredentials {
		return nil
	}

	var credentials map[string]any
	err := json.Unmarshal(credentialsBytes, &credentials)
	if err != nil {
		return nil
	}

	return credentials
}

func validateCredentialsChange(oldCredentials, newCredentials map[string]any) error {
	oldType, hasType := oldCredentials[credentialsTypeKey]
	if !hasType {
		oldType = korifiv1alpha1.UserProvidedType
	}

	newType, hasType := newCredentials[credentialsTypeKey]
	if !hasType {
		return nil
	}

	if !reflect.DeepEqual(oldType, newType) {
		return apierrors.NewInvalidRequestError(
			fmt.Errorf("cannot modify credential type: currently '%v': updating to '%v'", oldType, newType),
			"Cannot change credential type. Consider creating a new Service Instance.",
		)
	}

	return nil
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

	preds := []func(korifiv1alpha1.CFServiceInstance) bool{
		SetPredicate(message.Names, func(s korifiv1alpha1.CFServiceInstance) string { return s.Spec.DisplayName }),
		SetPredicate(message.GUIDs, func(s korifiv1alpha1.CFServiceInstance) string { return s.Name }),
	}

	labelSelector, err := labels.Parse(message.LabelSelector)
	if err != nil {
		return []ServiceInstanceRecord{}, apierrors.NewUnprocessableEntityError(err, "invalid label selector")
	}

	spaceGUIDSet := NewSet(message.SpaceGUIDs...)
	var filteredServiceInstances []korifiv1alpha1.CFServiceInstance
	for ns := range nsList {
		if len(spaceGUIDSet) > 0 && !spaceGUIDSet.Includes(ns) {
			continue
		}

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
		filteredServiceInstances = append(filteredServiceInstances, Filter(serviceInstanceList.Items, preds...)...)
	}

	return returnServiceInstanceList(filteredServiceInstances)
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

	return cfServiceInstanceToServiceInstanceRecord(serviceInstance)
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

func (m CreateServiceInstanceMessage) toCFServiceInstance() (*korifiv1alpha1.CFServiceInstance, error) {
	guid := uuid.NewString()

	instance := &korifiv1alpha1.CFServiceInstance{
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

	if len(m.Parameters) > 0 {
		rawParams, err := json.Marshal(m.Parameters)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal parameters: %w", err)
		}
		instance.Spec.Parameters = &runtime.RawExtension{Raw: rawParams}
	}

	return instance, nil
}

func cfServiceInstanceToServiceInstanceRecord(cfServiceInstance *korifiv1alpha1.CFServiceInstance) (ServiceInstanceRecord, error) {
	parameters := map[string]any{}
	if cfServiceInstance.Spec.Parameters != nil {
		err := json.Unmarshal(cfServiceInstance.Spec.Parameters.Raw, &parameters)
		if err != nil {
			return ServiceInstanceRecord{}, fmt.Errorf("failed to unmarshal service parameters: %w", err)
		}
	}

	record := ServiceInstanceRecord{
		Name:        cfServiceInstance.Spec.DisplayName,
		GUID:        cfServiceInstance.Name,
		SpaceGUID:   cfServiceInstance.Namespace,
		SecretName:  cfServiceInstance.Spec.SecretName,
		Tags:        cfServiceInstance.Spec.Tags,
		Type:        string(cfServiceInstance.Spec.Type),
		PlanGUID:    cfServiceInstance.Spec.ServicePlanGUID,
		Labels:      cfServiceInstance.Labels,
		Annotations: cfServiceInstance.Annotations,
		CreatedAt:   cfServiceInstance.CreationTimestamp.Time,
		UpdatedAt:   getLastUpdatedTime(cfServiceInstance),
		Parameters:  parameters,
	}

	if meta.IsStatusConditionTrue(cfServiceInstance.Status.Conditions, services.ReadyCondition) {
		record.State = tools.PtrTo(korifiv1alpha1.CFResourceState{
			Status:  korifiv1alpha1.ReadyStatus,
			Details: "service instance is ready",
		})
	}
	if meta.IsStatusConditionTrue(cfServiceInstance.Status.Conditions, services.FailedCondition) {
		record.State = tools.PtrTo(korifiv1alpha1.CFResourceState{
			Status:  korifiv1alpha1.FailedStatus,
			Details: meta.FindStatusCondition(cfServiceInstance.Status.Conditions, services.FailedCondition).Message,
		})
	}

	return record, nil
}

func returnServiceInstanceList(serviceInstanceList []korifiv1alpha1.CFServiceInstance) ([]ServiceInstanceRecord, error) {
	serviceInstanceRecords := make([]ServiceInstanceRecord, 0, len(serviceInstanceList))

	for i := range serviceInstanceList {
		record, err := cfServiceInstanceToServiceInstanceRecord(&serviceInstanceList[i])
		if err != nil {
			return nil, err
		}
		serviceInstanceRecords = append(serviceInstanceRecords, record)
	}
	return serviceInstanceRecords, nil
}
