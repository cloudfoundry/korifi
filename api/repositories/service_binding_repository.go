package repositories

import (
	"context"
	"fmt"
	"k8s.io/apimachinery/pkg/api/meta"
	"time"

	"code.cloudfoundry.org/korifi/api/apierrors"
	"code.cloudfoundry.org/korifi/api/authorization"
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/webhooks"
	"code.cloudfoundry.org/korifi/controllers/webhooks/services"

	"github.com/google/uuid"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	LabelServiceBindingProvisionedService = "servicebinding.io/provisioned-service"
	ServiceBindingResourceType            = "Service Binding"
	ServiceBindingTypeApp                 = "app"
)

type ServiceBindingRepo struct {
	userClientFactory    authorization.UserK8sClientFactory
	namespacePermissions *authorization.NamespacePermissions
	namespaceRetriever   NamespaceRetriever
	timeout              time.Duration
}

func NewServiceBindingRepo(
	namespaceRetriever NamespaceRetriever,
	userClientFactory authorization.UserK8sClientFactory,
	namespacePermissions *authorization.NamespacePermissions,
	timeout time.Duration,
) *ServiceBindingRepo {
	return &ServiceBindingRepo{
		userClientFactory:    userClientFactory,
		namespacePermissions: namespacePermissions,
		namespaceRetriever:   namespaceRetriever,
		timeout:              timeout,
	}
}

type ServiceBindingRecord struct {
	GUID                string
	Type                string
	Name                *string
	AppGUID             string
	ServiceInstanceGUID string
	SpaceGUID           string
	CreatedAt           string
	UpdatedAt           string
	LastOperation       ServiceBindingLastOperation
}

type ServiceBindingLastOperation struct {
	Type        string
	State       string
	Description *string
	CreatedAt   string
	UpdatedAt   string
}

type CreateServiceBindingMessage struct {
	Name                *string
	ServiceInstanceGUID string
	AppGUID             string
	SpaceGUID           string
}

type DeleteServiceBindingMessage struct {
	GUID string
}

type ListServiceBindingsMessage struct {
	AppGUIDs             []string
	ServiceInstanceGUIDs []string
}

func (m CreateServiceBindingMessage) toCFServiceBinding() korifiv1alpha1.CFServiceBinding {
	guid := uuid.NewString()
	return korifiv1alpha1.CFServiceBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      guid,
			Namespace: m.SpaceGUID,
			Labels:    map[string]string{LabelServiceBindingProvisionedService: "true"},
		},
		Spec: korifiv1alpha1.CFServiceBindingSpec{
			DisplayName: m.Name,
			Service: corev1.ObjectReference{
				Kind:       "CFServiceInstance",
				APIVersion: korifiv1alpha1.GroupVersion.Identifier(),
				Name:       m.ServiceInstanceGUID,
			},
			AppRef: corev1.LocalObjectReference{Name: m.AppGUID},
		},
	}
}

func (r *ServiceBindingRepo) CreateServiceBinding(ctx context.Context, authInfo authorization.Info, message CreateServiceBindingMessage) (ServiceBindingRecord, error) {
	userClient, err := r.userClientFactory.BuildClient(authInfo)
	if err != nil {
		return ServiceBindingRecord{}, fmt.Errorf("failed to build user client: %w", err)
	}

	cfServiceBinding := message.toCFServiceBinding()
	err = userClient.Create(ctx, &cfServiceBinding)
	if err != nil {
		if validationError, ok := webhooks.WebhookErrorToValidationError(err); ok {
			if validationError.Type == services.ServiceBindingErrorType {
				return ServiceBindingRecord{}, apierrors.NewUniquenessError(err, validationError.GetMessage())
			}
		}

		return ServiceBindingRecord{}, apierrors.FromK8sError(err, ServiceBindingResourceType)
	}

	timeoutCtx, cancelFn := context.WithTimeout(ctx, r.timeout)
	defer cancelFn()
	watch, err := userClient.Watch(timeoutCtx, &korifiv1alpha1.CFServiceBindingList{},
		client.InNamespace(cfServiceBinding.Namespace),
		client.MatchingFields{"metadata.name": cfServiceBinding.Name},
	)
	if err != nil {
		return ServiceBindingRecord{}, fmt.Errorf("failed to set up watch on service binding: %w", apierrors.FromK8sError(err, ServiceBindingResourceType))
	}

	conditionReady := false
	var createdServiceBinding *korifiv1alpha1.CFServiceBinding
	for res := range watch.ResultChan() {
		var ok bool
		createdServiceBinding, ok = res.Object.(*korifiv1alpha1.CFServiceBinding)
		if !ok {
			// should never happen, but avoids panic above
			continue
		}
		if meta.IsStatusConditionTrue(createdServiceBinding.Status.Conditions, VCAPServicesSecretAvailableCondition) {
			watch.Stop()
			conditionReady = true
			break
		}
	}

	if !conditionReady {
		return ServiceBindingRecord{}, fmt.Errorf("service binding did not get Condition `VCAPServicesSecretAvailable`: 'True' within timeout period %d ms", r.timeout.Milliseconds())
	}

	return cfServiceBindingToRecord(cfServiceBinding), err
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

func cfServiceBindingToRecord(binding korifiv1alpha1.CFServiceBinding) ServiceBindingRecord {
	createdAt := binding.CreationTimestamp.UTC().Format(TimestampFormat)
	updatedAt, _ := getTimeLastUpdatedTimestamp(&binding.ObjectMeta)
	return ServiceBindingRecord{
		GUID:                binding.Name,
		Type:                ServiceBindingTypeApp,
		Name:                binding.Spec.DisplayName,
		AppGUID:             binding.Spec.AppRef.Name,
		ServiceInstanceGUID: binding.Spec.Service.Name,
		SpaceGUID:           binding.Namespace,
		CreatedAt:           createdAt,
		UpdatedAt:           updatedAt,
		LastOperation: ServiceBindingLastOperation{
			Type:        "create",
			State:       "succeeded",
			Description: nil,
			CreatedAt:   createdAt,
			UpdatedAt:   updatedAt,
		},
	}
}

func (r *ServiceBindingRepo) ListServiceBindings(ctx context.Context, authInfo authorization.Info, message ListServiceBindingsMessage) ([]ServiceBindingRecord, error) {
	nsList, err := r.namespacePermissions.GetAuthorizedSpaceNamespaces(ctx, authInfo)
	if err != nil {
		return nil, fmt.Errorf("failed to list namespaces for spaces with user role bindings: %w", err)
	}

	userClient, err := r.userClientFactory.BuildClient(authInfo)
	if err != nil {
		return []ServiceBindingRecord{}, fmt.Errorf("failed to build user client: %w", err)
	}

	var filteredServiceBindings []korifiv1alpha1.CFServiceBinding
	for ns := range nsList {
		serviceInstanceList := new(korifiv1alpha1.CFServiceBindingList)
		err = userClient.List(ctx, serviceInstanceList, client.InNamespace(ns))
		if k8serrors.IsForbidden(err) {
			continue
		}
		if err != nil {
			return []ServiceBindingRecord{}, fmt.Errorf("failed to list service instances in namespace %s: %w",
				ns,
				apierrors.FromK8sError(err, ServiceBindingResourceType),
			)
		}
		filteredServiceBindings = append(filteredServiceBindings, applyServiceBindingListFilter(serviceInstanceList.Items, message)...)
	}

	return toServiceBindingRecords(filteredServiceBindings), nil
}

func applyServiceBindingListFilter(serviceBindingList []korifiv1alpha1.CFServiceBinding, message ListServiceBindingsMessage) []korifiv1alpha1.CFServiceBinding {
	var filtered []korifiv1alpha1.CFServiceBinding
	for _, serviceBinding := range serviceBindingList {
		if matchesFilter(serviceBinding.Spec.Service.Name, message.ServiceInstanceGUIDs) &&
			matchesFilter(serviceBinding.Spec.AppRef.Name, message.AppGUIDs) {
			filtered = append(filtered, serviceBinding)
		}
	}

	return filtered
}

func toServiceBindingRecords(serviceInstanceList []korifiv1alpha1.CFServiceBinding) []ServiceBindingRecord {
	serviceInstanceRecords := make([]ServiceBindingRecord, 0, len(serviceInstanceList))

	for _, serviceInstance := range serviceInstanceList {
		serviceInstanceRecords = append(serviceInstanceRecords, cfServiceBindingToRecord(serviceInstance))
	}
	return serviceInstanceRecords
}
