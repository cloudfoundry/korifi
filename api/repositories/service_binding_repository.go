package repositories

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"k8s.io/client-go/kubernetes/scheme"

	"code.cloudfoundry.org/korifi/api/authorization"
	apierrors "code.cloudfoundry.org/korifi/api/errors"
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/webhooks/services/bindings"
	"code.cloudfoundry.org/korifi/controllers/webhooks/validation"
	"code.cloudfoundry.org/korifi/tools"

	"github.com/BooleanCat/go-functional/v2/it"
	"github.com/BooleanCat/go-functional/v2/it/itx"
	"github.com/google/uuid"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	LabelServiceBindingProvisionedService = "servicebinding.io/provisioned-service"
	ServiceBindingResourceType            = "Service Binding"
)

type ParametersClient interface {
	GetServiceBindingParameters(ctx context.Context, serviceBinding *korifiv1alpha1.CFServiceBinding) (map[string]any, error)
}

type ServiceBindingRepo struct {
	klient                  Klient
	bindingConditionAwaiter Awaiter[*korifiv1alpha1.CFServiceBinding]
	appConditionAwaiter     Awaiter[*korifiv1alpha1.CFApp]
	paramsClient            ParametersClient
}

func NewServiceBindingRepo(
	klient Klient,
	bindingConditionAwaiter Awaiter[*korifiv1alpha1.CFServiceBinding],
	appConditionAwaiter Awaiter[*korifiv1alpha1.CFApp],
	paramsClient ParametersClient,
) *ServiceBindingRepo {
	return &ServiceBindingRepo{
		klient:                  klient,
		bindingConditionAwaiter: bindingConditionAwaiter,
		appConditionAwaiter:     appConditionAwaiter,
		paramsClient:            paramsClient,
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
	if message.Type == korifiv1alpha1.CFServiceBindingTypeApp {
		return r.createAppServiceBinding(ctx, message)
	}

	return r.createServiceBinding(ctx, message)
}

func (r *ServiceBindingRepo) createAppServiceBinding(ctx context.Context, message CreateServiceBindingMessage) (ServiceBindingRecord, error) {
	cfApp := &korifiv1alpha1.CFApp{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: message.SpaceGUID,
			Name:      message.AppGUID,
		},
	}
	err := r.klient.Get(ctx, cfApp)
	if err != nil {
		return ServiceBindingRecord{},
			apierrors.AsUnprocessableEntity(
				apierrors.FromK8sError(err, ServiceBindingResourceType),
				"Unable to use app. Ensure that the app exists and you have access to it.",
				apierrors.ForbiddenError{},
				apierrors.NotFoundError{},
			)
	}

	bindingRecord, err := r.createServiceBinding(ctx, message)
	if err != nil {
		return ServiceBindingRecord{}, err
	}

	_, err = r.appConditionAwaiter.AwaitState(ctx, r.klient, cfApp, func(a *korifiv1alpha1.CFApp) error {
		if a.Generation != a.Status.ObservedGeneration {
			return fmt.Errorf("app status is outdated")
		}

		if !slices.Contains(actualBindingGUIDs(a), bindingRecord.GUID) {
			return fmt.Errorf("binding %q not available in cf app status", bindingRecord.GUID)
		}

		return nil
	})
	if err != nil {
		return ServiceBindingRecord{}, err
	}

	return bindingRecord, nil
}

func (r *ServiceBindingRepo) createServiceBinding(ctx context.Context, message CreateServiceBindingMessage) (ServiceBindingRecord, error) {
	cfServiceInstance := &korifiv1alpha1.CFServiceInstance{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: message.SpaceGUID,
			Name:      message.ServiceInstanceGUID,
		},
	}
	err := r.klient.Get(ctx, cfServiceInstance)
	if err != nil {
		return ServiceBindingRecord{},
			apierrors.AsUnprocessableEntity(
				apierrors.FromK8sError(err, ServiceBindingResourceType),
				"Unable to bind to instance. Ensure that the instance exists and you have access to it.",
				apierrors.ForbiddenError{},
				apierrors.NotFoundError{},
			)
	}

	cfServiceBinding := message.toCFServiceBinding(cfServiceInstance.Spec.Type)
	err = r.klient.Create(ctx, cfServiceBinding)
	if err != nil {
		if validationError, ok := validation.WebhookErrorToValidationError(err); ok {
			if validationError.Type == bindings.ServiceBindingErrorType {
				return ServiceBindingRecord{}, apierrors.NewUniquenessError(err, validationError.GetMessage())
			}
		}

		return ServiceBindingRecord{}, apierrors.FromK8sError(err, ServiceBindingResourceType)
	}

	if cfServiceInstance.Spec.Type == korifiv1alpha1.ManagedType {
		err = r.createParametersSecret(ctx, cfServiceBinding, message.Parameters)
		if err != nil {
			return ServiceBindingRecord{}, apierrors.FromK8sError(err, ServiceBindingResourceType)
		}
	}

	if cfServiceInstance.Spec.Type == korifiv1alpha1.UserProvidedType {
		cfServiceBinding, err = r.bindingConditionAwaiter.AwaitCondition(ctx, r.klient, cfServiceBinding, korifiv1alpha1.StatusConditionReady)
		if err != nil {
			return ServiceBindingRecord{}, err
		}
	}

	return serviceBindingToRecord(*cfServiceBinding), nil
}

func actualBindingGUIDs(cfApp *korifiv1alpha1.CFApp) []string {
	return slices.Collect(it.Map(slices.Values(cfApp.Status.ServiceBindings), func(b korifiv1alpha1.ServiceBinding) string {
		return b.GUID
	}))
}

func (r *ServiceBindingRepo) createParametersSecret(ctx context.Context, cfServiceBinding *korifiv1alpha1.CFServiceBinding, parameters map[string]any) error {
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

	return r.klient.Create(ctx, paramsSecret)
}

func (r *ServiceBindingRepo) DeleteServiceBinding(ctx context.Context, authInfo authorization.Info, guid string) error {
	binding := &korifiv1alpha1.CFServiceBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: guid,
		},
	}

	err := r.klient.Get(ctx, binding)
	if err != nil {
		return apierrors.ForbiddenAsNotFound(apierrors.FromK8sError(err, ServiceBindingResourceType))
	}

	err = r.klient.Delete(ctx, binding)
	if err != nil {
		return apierrors.FromK8sError(err, ServiceBindingResourceType)
	}

	cfApp := &korifiv1alpha1.CFApp{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: binding.Namespace,
			Name:      binding.Spec.AppRef.Name,
		},
	}
	err = r.klient.Get(ctx, cfApp)
	if err != nil {
		return apierrors.FromK8sError(err, ServiceBindingResourceType)
	}

	_, err = r.appConditionAwaiter.AwaitState(ctx, r.klient, cfApp, func(a *korifiv1alpha1.CFApp) error {
		if a.Generation != a.Status.ObservedGeneration {
			return fmt.Errorf("app status is outdated")
		}

		if slices.Contains(actualBindingGUIDs(a), binding.Name) {
			return fmt.Errorf("binding %q is still available in cf app status", binding.Name)
		}

		return nil
	})
	return err
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

	credentialsSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: binding.Namespace,
			Name:      binding.Status.EnvSecretRef.Name,
		},
	}
	err = r.klient.Get(ctx, credentialsSecret)
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
	serviceBinding := korifiv1alpha1.CFServiceBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: bindingGUID,
		},
	}
	err := r.klient.Get(ctx, &serviceBinding)
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
	serviceBinding := &korifiv1alpha1.CFServiceBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: updateMsg.GUID,
		},
	}

	err := r.klient.Get(ctx, serviceBinding)
	if err != nil {
		return ServiceBindingRecord{}, fmt.Errorf("failed to get service binding: %w", apierrors.FromK8sError(err, ServiceBindingResourceType))
	}

	err = r.klient.Patch(ctx, serviceBinding, func() error {
		updateMsg.MetadataPatch.Apply(serviceBinding)
		return nil
	})
	if err != nil {
		return ServiceBindingRecord{}, fmt.Errorf("failed to patch service binding metadata: %w", apierrors.FromK8sError(err, ServiceBindingResourceType))
	}

	return serviceBindingToRecord(*serviceBinding), nil
}

func (r *ServiceBindingRepo) GetState(ctx context.Context, authInfo authorization.Info, guid string) (ResourceState, error) {
	bindingRecord, err := r.GetServiceBinding(ctx, authInfo, guid)
	if err != nil {
		return ResourceStateUnknown, err
	}

	if bindingRecord.Ready {
		return ResourceStateReady, nil
	}

	return ResourceStateUnknown, nil
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
	serviceBindingList := new(korifiv1alpha1.CFServiceBindingList)
	err := r.klient.List(ctx, serviceBindingList, WithLabelSelector(message.LabelSelector))
	if err != nil {
		return []ServiceBindingRecord{}, fmt.Errorf("failed to list service instances: %w",
			apierrors.FromK8sError(err, ServiceBindingResourceType),
		)
	}

	filteredServiceBindings := itx.FromSlice(serviceBindingList.Items).Filter(message.matches)
	return slices.Collect(it.Map(filteredServiceBindings, serviceBindingToRecord)), nil
}

func (r *ServiceBindingRepo) GetServiceBindingParameters(ctx context.Context, authInfo authorization.Info, guid string) (map[string]any, error) {
	serviceBinding, err := r.getServiceBinding(ctx, authInfo, guid)
	if err != nil {
		return map[string]any{}, fmt.Errorf("get-service-binding failed: %w", err)
	}

	params, err := r.paramsClient.GetServiceBindingParameters(ctx, &serviceBinding)
	if err != nil {
		return map[string]any{}, err
	}

	return params, nil
}
