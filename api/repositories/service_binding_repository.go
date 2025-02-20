package repositories

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"

	"code.cloudfoundry.org/korifi/api/authorization"
	apierrors "code.cloudfoundry.org/korifi/api/errors"
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/webhooks/services/bindings"
	"code.cloudfoundry.org/korifi/controllers/webhooks/validation"
	"code.cloudfoundry.org/korifi/model"
	"code.cloudfoundry.org/korifi/tools"
	"code.cloudfoundry.org/korifi/tools/k8s"

	"github.com/BooleanCat/go-functional/v2/it"
	"github.com/BooleanCat/go-functional/v2/it/itx"
	"github.com/google/uuid"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	LabelServiceBindingProvisionedService = "servicebinding.io/provisioned-service"
	ServiceBindingResourceType            = "Service Binding"
)

type ServiceBindingRepo struct {
	userClientFactory       authorization.UserClientFactory
	namespaceRetriever      NamespaceRetriever
	bindingConditionAwaiter Awaiter[*korifiv1alpha1.CFServiceBinding]
}

func NewServiceBindingRepo(
	namespaceRetriever NamespaceRetriever,
	userClientFactory authorization.UserClientFactory,
	bindingConditionAwaiter Awaiter[*korifiv1alpha1.CFServiceBinding],
) *ServiceBindingRepo {
	return &ServiceBindingRepo{
		userClientFactory:       userClientFactory,
		namespaceRetriever:      namespaceRetriever,
		bindingConditionAwaiter: bindingConditionAwaiter,
	}
}

type ServiceBindingRecord struct {
	GUID                string
	Type                string
	Name                *string
	AppGUID             string
	ServiceInstanceGUID string
	SpaceGUID           string
	Labels              map[string]string
	Annotations         map[string]string
	CreatedAt           time.Time
	UpdatedAt           *time.Time
	DeletedAt           *time.Time
	LastOperation       ServiceBindingLastOperation
	Ready               bool
}

func (r ServiceBindingRecord) Relationships() map[string]string {
	return map[string]string{
		"app":              r.AppGUID,
		"service_instance": r.ServiceInstanceGUID,
	}
}

type ServiceBindingDetailsRecord struct {
	Credentials map[string]any
}
type ServiceBindingLastOperation struct {
	Type        string
	State       string
	Description *string
	CreatedAt   time.Time
	UpdatedAt   *time.Time
}

type CreateServiceBindingMessage struct {
	Type                string
	Name                *string
	ServiceInstanceGUID string
	AppGUID             string
	SpaceGUID           string
	Parameters          map[string]any
}

type DeleteServiceBindingMessage struct {
	GUID string
}

type ListServiceBindingsMessage struct {
	AppGUIDs             []string
	ServiceInstanceGUIDs []string
	LabelSelector        string
	Type                 string
	PlanGUIDs            []string
}

func (m *ListServiceBindingsMessage) matches(serviceBinding korifiv1alpha1.CFServiceBinding) bool {
	return tools.EmptyOrContains(m.ServiceInstanceGUIDs, serviceBinding.Spec.Service.Name) &&
		tools.EmptyOrContains(m.AppGUIDs, serviceBinding.Spec.AppRef.Name) &&
		tools.EmptyOrContains(m.PlanGUIDs, serviceBinding.Labels[korifiv1alpha1.PlanGUIDLabelKey]) &&
		tools.ZeroOrEquals(m.Type, serviceBinding.Spec.Type)
}

func (m CreateServiceBindingMessage) toCFServiceBinding(instanceType korifiv1alpha1.InstanceType) *korifiv1alpha1.CFServiceBinding {
	binding := &korifiv1alpha1.CFServiceBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      uuid.NewString(),
			Namespace: m.SpaceGUID,
			Labels: map[string]string{
				LabelServiceBindingProvisionedService: "true",
			},
		},
		Spec: korifiv1alpha1.CFServiceBindingSpec{
			DisplayName: m.Name,
			Service: corev1.ObjectReference{
				Kind:       "CFServiceInstance",
				APIVersion: korifiv1alpha1.SchemeGroupVersion.Identifier(),
				Name:       m.ServiceInstanceGUID,
			},
			Type: m.Type,
		},
	}

	if instanceType == korifiv1alpha1.ManagedType {
		binding.Spec.Parameters.Name = uuid.NewString()
	}

	if m.Type == "app" {
		binding.Spec.AppRef = corev1.LocalObjectReference{Name: m.AppGUID}
	}

	return binding
}

type UpdateServiceBindingMessage struct {
	GUID          string
	MetadataPatch MetadataPatch
}

func (r *ServiceBindingRepo) CreateServiceBinding(ctx context.Context, authInfo authorization.Info, message CreateServiceBindingMessage) (ServiceBindingRecord, error) {
	userClient, err := r.userClientFactory.BuildClient(authInfo)
	if err != nil {
		return ServiceBindingRecord{}, fmt.Errorf("failed to build user client: %w", err)
	}

	cfServiceInstance := new(korifiv1alpha1.CFServiceInstance)
	err = userClient.Get(ctx, types.NamespacedName{Name: message.ServiceInstanceGUID, Namespace: message.SpaceGUID}, cfServiceInstance)
	if err != nil {
		return ServiceBindingRecord{},
			apierrors.AsUnprocessableEntity(
				apierrors.FromK8sError(err, ServiceBindingResourceType),
				"Unable to bind to instance. Ensure that the instance exists and you have access to it.",
				apierrors.ForbiddenError{},
				apierrors.NotFoundError{},
			)
	}

	if message.Type == korifiv1alpha1.CFServiceBindingTypeApp {
		cfApp := new(korifiv1alpha1.CFApp)
		err = userClient.Get(ctx, types.NamespacedName{Name: message.AppGUID, Namespace: message.SpaceGUID}, cfApp)
		if err != nil {
			return ServiceBindingRecord{},
				apierrors.AsUnprocessableEntity(
					apierrors.FromK8sError(err, ServiceBindingResourceType),
					"Unable to use app. Ensure that the app exists and you have access to it.",
					apierrors.ForbiddenError{},
					apierrors.NotFoundError{},
				)
		}
	}

	cfServiceBinding := message.toCFServiceBinding(cfServiceInstance.Spec.Type)
	err = userClient.Create(ctx, cfServiceBinding)
	if err != nil {
		if validationError, ok := validation.WebhookErrorToValidationError(err); ok {
			if validationError.Type == bindings.ServiceBindingErrorType {
				return ServiceBindingRecord{}, apierrors.NewUniquenessError(err, validationError.GetMessage())
			}
		}

		return ServiceBindingRecord{}, apierrors.FromK8sError(err, ServiceBindingResourceType)
	}

	if cfServiceInstance.Spec.Type == korifiv1alpha1.ManagedType {
		err = r.createParametersSecret(ctx, userClient, cfServiceBinding, message.Parameters)
		if err != nil {
			return ServiceBindingRecord{}, apierrors.FromK8sError(err, ServiceBindingResourceType)
		}
	}

	if cfServiceInstance.Spec.Type == korifiv1alpha1.UserProvidedType {
		cfServiceBinding, err = r.bindingConditionAwaiter.AwaitCondition(ctx, userClient, cfServiceBinding, korifiv1alpha1.StatusConditionReady)
		if err != nil {
			return ServiceBindingRecord{}, err
		}
	}

	return serviceBindingToRecord(*cfServiceBinding), nil
}

func (r *ServiceBindingRepo) createParametersSecret(ctx context.Context, userClient client.Client, cfServiceBinding *korifiv1alpha1.CFServiceBinding, parameters map[string]any) error {
	parametersData, err := tools.ToParametersSecretData(parameters)
	if err != nil {
		return err
	}

	paramsSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: cfServiceBinding.Namespace,
			Name:      cfServiceBinding.Spec.Parameters.Name,
		},
		Data: parametersData,
	}

	_ = controllerutil.SetOwnerReference(cfServiceBinding, paramsSecret, scheme.Scheme)

	return userClient.Create(ctx, paramsSecret)
}

func (r *ServiceBindingRepo) DeleteServiceBinding(ctx context.Context, authInfo authorization.Info, guid string) error {
	userClient, err := r.userClientFactory.BuildClient(authInfo)
	if err != nil {
		return fmt.Errorf("failed to build user client: %w", err)
	}

	namespace, err := r.namespaceRetriever.NamespaceFor(ctx, guid, ServiceBindingResourceType)
	if err != nil {
		return err
	}

	binding := &korifiv1alpha1.CFServiceBinding{}

	err = userClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: guid}, binding)
	if err != nil {
		return apierrors.ForbiddenAsNotFound(apierrors.FromK8sError(err, ServiceBindingResourceType))
	}

	err = userClient.Delete(ctx, binding)
	if err != nil {
		return apierrors.FromK8sError(err, ServiceBindingResourceType)
	}
	return nil
}

func (r *ServiceBindingRepo) GetServiceBinding(ctx context.Context, authInfo authorization.Info, guid string) (ServiceBindingRecord, error) {
	binding, err := r.getServiceBinding(ctx, authInfo, guid)
	if err != nil {
		return ServiceBindingRecord{}, fmt.Errorf("get-service-binding failed: %w", err)
	}

	return serviceBindingToRecord(binding), nil
}

func (r *ServiceBindingRepo) GetServiceBindingDetails(ctx context.Context, authInfo authorization.Info, guid string) (ServiceBindingDetailsRecord, error) {
	binding, err := r.getServiceBinding(ctx, authInfo, guid)
	if err != nil {
		return ServiceBindingDetailsRecord{}, fmt.Errorf("get-service-binding-details failed: %w", err)
	}
	if ok := isBindingReady(binding); !ok {
		return ServiceBindingDetailsRecord{}, errors.New("get-service-binding-details failed due to binding not ready yet")
	}

	userClient, err := r.userClientFactory.BuildClient(authInfo)
	if err != nil {
		return ServiceBindingDetailsRecord{}, fmt.Errorf("get-service-binding-details failed to create user client: %w", err)
	}

	credentialsSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: binding.Namespace,
			Name:      binding.Status.EnvSecretRef.Name,
		},
	}
	err = userClient.Get(ctx, client.ObjectKeyFromObject(credentialsSecret), credentialsSecret)
	if err != nil {
		return ServiceBindingDetailsRecord{}, fmt.Errorf("failed to get credentials: %w", apierrors.FromK8sError(err, ServiceBindingResourceType))
	}

	credentials, err := tools.FromCredentialsSecretData(credentialsSecret.Data)
	if err != nil {
		return ServiceBindingDetailsRecord{}, fmt.Errorf("failed to parse credentials secret data: %w", err)
	}

	return ServiceBindingDetailsRecord{Credentials: credentials}, nil
}

func (r *ServiceBindingRepo) getServiceBinding(ctx context.Context, authInfo authorization.Info, bindingGUID string) (korifiv1alpha1.CFServiceBinding, error) {
	namespace, err := r.namespaceRetriever.NamespaceFor(ctx, bindingGUID, ServiceBindingResourceType)
	if err != nil {
		return korifiv1alpha1.CFServiceBinding{}, fmt.Errorf("failed to retrieve namespace: %w", err)
	}

	userClient, err := r.userClientFactory.BuildClient(authInfo)
	if err != nil {
		return korifiv1alpha1.CFServiceBinding{}, fmt.Errorf("failed to build user client: %w", err)
	}

	serviceBinding := korifiv1alpha1.CFServiceBinding{}
	err = userClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: bindingGUID}, &serviceBinding)
	if err != nil {
		return korifiv1alpha1.CFServiceBinding{}, fmt.Errorf("failed to get service binding: %w", apierrors.FromK8sError(err, ServiceBindingResourceType))
	}

	return serviceBinding, nil
}

func serviceBindingToRecord(binding korifiv1alpha1.CFServiceBinding) ServiceBindingRecord {
	return ServiceBindingRecord{
		GUID:                binding.Name,
		Type:                binding.Spec.Type,
		Name:                binding.Spec.DisplayName,
		AppGUID:             binding.Spec.AppRef.Name,
		ServiceInstanceGUID: binding.Spec.Service.Name,
		SpaceGUID:           binding.Namespace,
		Labels:              binding.Labels,
		Annotations:         binding.Annotations,
		CreatedAt:           binding.CreationTimestamp.Time,
		UpdatedAt:           getLastUpdatedTime(&binding),
		DeletedAt:           golangTime(binding.DeletionTimestamp),
		LastOperation:       serviceBindingRecordLastOperation(binding),
		Ready:               isBindingReady(binding),
	}
}

func isBindingReady(binding korifiv1alpha1.CFServiceBinding) bool {
	if binding.Generation != binding.Status.ObservedGeneration {
		return false
	}

	return meta.IsStatusConditionTrue(binding.Status.Conditions, korifiv1alpha1.StatusConditionReady)
}

func serviceBindingRecordLastOperation(binding korifiv1alpha1.CFServiceBinding) ServiceBindingLastOperation {
	if binding.DeletionTimestamp != nil {
		return ServiceBindingLastOperation{
			Type:      "delete",
			State:     "in progress",
			CreatedAt: binding.DeletionTimestamp.Time,
			UpdatedAt: getLastUpdatedTime(&binding),
		}
	}

	readyCondition := meta.FindStatusCondition(binding.Status.Conditions, korifiv1alpha1.StatusConditionReady)
	if readyCondition == nil {
		return ServiceBindingLastOperation{
			Type:      "create",
			State:     "initial",
			CreatedAt: binding.CreationTimestamp.Time,
			UpdatedAt: getLastUpdatedTime(&binding),
		}
	}

	if readyCondition.Status == metav1.ConditionTrue {
		return ServiceBindingLastOperation{
			Type:      "create",
			State:     "succeeded",
			CreatedAt: binding.CreationTimestamp.Time,
			UpdatedAt: getLastUpdatedTime(&binding),
		}
	}

	if meta.IsStatusConditionTrue(binding.Status.Conditions, korifiv1alpha1.BindingFailedCondition) {
		return ServiceBindingLastOperation{
			Type:      "create",
			State:     "failed",
			CreatedAt: binding.CreationTimestamp.Time,
			UpdatedAt: tools.PtrTo(readyCondition.LastTransitionTime.Time),
		}
	}

	return ServiceBindingLastOperation{
		Type:      "create",
		State:     "in progress",
		CreatedAt: binding.CreationTimestamp.Time,
		UpdatedAt: tools.PtrTo(readyCondition.LastTransitionTime.Time),
	}
}

func (r *ServiceBindingRepo) UpdateServiceBinding(ctx context.Context, authInfo authorization.Info, updateMsg UpdateServiceBindingMessage) (ServiceBindingRecord, error) {
	userClient, err := r.userClientFactory.BuildClient(authInfo)
	if err != nil {
		return ServiceBindingRecord{}, fmt.Errorf("failed to create user client: %w", err)
	}

	ns, err := r.namespaceRetriever.NamespaceFor(ctx, updateMsg.GUID, ServiceBindingResourceType)
	if err != nil {
		return ServiceBindingRecord{}, err
	}

	serviceBinding := &korifiv1alpha1.CFServiceBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      updateMsg.GUID,
			Namespace: ns,
		},
	}

	err = userClient.Get(ctx, client.ObjectKeyFromObject(serviceBinding), serviceBinding)
	if err != nil {
		return ServiceBindingRecord{}, fmt.Errorf("failed to get service binding: %w", apierrors.FromK8sError(err, ServiceBindingResourceType))
	}

	err = k8s.PatchResource(ctx, userClient, serviceBinding, func() {
		updateMsg.MetadataPatch.Apply(serviceBinding)
	})
	if err != nil {
		return ServiceBindingRecord{}, fmt.Errorf("failed to patch service binding metadata: %w", apierrors.FromK8sError(err, ServiceBindingResourceType))
	}

	return serviceBindingToRecord(*serviceBinding), nil
}

func (r *ServiceBindingRepo) GetState(ctx context.Context, authInfo authorization.Info, guid string) (model.CFResourceState, error) {
	bindingRecord, err := r.GetServiceBinding(ctx, authInfo, guid)
	if err != nil {
		return model.CFResourceStateUnknown, err
	}

	if bindingRecord.Ready {
		return model.CFResourceStateReady, nil
	}

	return model.CFResourceStateUnknown, nil
}

func (r *ServiceBindingRepo) GetDeletedAt(ctx context.Context, authInfo authorization.Info, bindingGUID string) (*time.Time, error) {
	serviceBinding, err := r.GetServiceBinding(ctx, authInfo, bindingGUID)
	if err != nil {
		return nil, err
	}
	return serviceBinding.DeletedAt, nil
}

// nolint:dupl
func (r *ServiceBindingRepo) ListServiceBindings(ctx context.Context, authInfo authorization.Info, message ListServiceBindingsMessage) ([]ServiceBindingRecord, error) {
	userClient, err := r.userClientFactory.BuildClient(authInfo)
	if err != nil {
		return []ServiceBindingRecord{}, fmt.Errorf("failed to build user client: %w", err)
	}

	labelSelector, err := labels.Parse(message.LabelSelector)
	if err != nil {
		return []ServiceBindingRecord{}, apierrors.NewUnprocessableEntityError(err, "invalid label selector")
	}

	serviceBindingList := new(korifiv1alpha1.CFServiceBindingList)
	err = userClient.List(ctx, serviceBindingList, &client.ListOptions{LabelSelector: labelSelector})
	if err != nil {
		return []ServiceBindingRecord{}, fmt.Errorf("failed to list service instances: %w",
			apierrors.FromK8sError(err, ServiceBindingResourceType),
		)
	}

	filteredServiceBindings := itx.FromSlice(serviceBindingList.Items).Filter(message.matches)
	return slices.Collect(it.Map(filteredServiceBindings, serviceBindingToRecord)), nil
}
