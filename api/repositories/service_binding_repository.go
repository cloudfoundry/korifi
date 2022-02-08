package repositories

import (
	"context"

	"github.com/google/uuid"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"code.cloudfoundry.org/cf-k8s-controllers/api/authorization"
	"code.cloudfoundry.org/cf-k8s-controllers/controllers/apis/services/v1alpha1"
)

//+kubebuilder:rbac:groups=services.cloudfoundry.org,resources=cfservicebindings,verbs=list;create

const (
	LabelServiceBindingProvisionedService = "servicebinding.io/provisioned-service"
)

type ServiceBindingRepo struct {
	userClientFactory UserK8sClientFactory
}

func NewServiceBindingRepo(userClientFactory UserK8sClientFactory) *ServiceBindingRepo {
	return &ServiceBindingRepo{
		userClientFactory: userClientFactory,
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

func (m CreateServiceBindingMessage) toCFServiceBinding() v1alpha1.CFServiceBinding {
	guid := uuid.NewString()
	return v1alpha1.CFServiceBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      guid,
			Namespace: m.SpaceGUID,
			Labels:    map[string]string{LabelServiceBindingProvisionedService: "true"},
		},
		Spec: v1alpha1.CFServiceBindingSpec{
			Name: m.Name,
			Service: corev1.ObjectReference{
				Kind:       "CFServiceInstance",
				APIVersion: v1alpha1.GroupVersion.Identifier(),
				Name:       m.ServiceInstanceGUID,
			},
			AppRef: corev1.LocalObjectReference{Name: m.AppGUID},
		},
	}
}

func (r *ServiceBindingRepo) CreateServiceBinding(ctx context.Context, authInfo authorization.Info, message CreateServiceBindingMessage) (ServiceBindingRecord, error) {
	userClient, err := r.userClientFactory.BuildClient(authInfo)
	if err != nil {
		panic(err) // TODO
		// return ServiceBindingRecord{}, fmt.Errorf("failed to build user client: %w", err)
	}

	cfServiceBinding := message.toCFServiceBinding()
	err = userClient.Create(ctx, &cfServiceBinding)
	if err != nil {
		if apierrors.IsForbidden(err) {
			return ServiceBindingRecord{}, NewForbiddenError(err)
		}
		return ServiceBindingRecord{}, err // untested
	}

	return cfServiceBindingToRecord(cfServiceBinding), err
}

func (r *ServiceBindingRepo) ServiceBindingExists(ctx context.Context, authInfo authorization.Info, spaceGUID, appGUID, serviceInstanceGUID string) (bool, error) {
	userClient, err := r.userClientFactory.BuildClient(authInfo)
	if err != nil {
		panic(err) // TODO
		// return ServiceBindingRecord{}, fmt.Errorf("failed to build user client: %w", err)
	}

	serviceBindingList := new(v1alpha1.CFServiceBindingList)
	err = userClient.List(ctx, serviceBindingList, client.InNamespace(spaceGUID))
	if err != nil {
		if apierrors.IsForbidden(err) {
			return false, NewForbiddenError(err)
		}
		return false, err // untested
	}

	for _, serviceBinding := range serviceBindingList.Items {
		if serviceBinding.Spec.AppRef.Name == appGUID &&
			serviceBinding.Spec.Service.Name == serviceInstanceGUID {
			return true, nil
		}
	}

	return false, nil
}

func cfServiceBindingToRecord(binding v1alpha1.CFServiceBinding) ServiceBindingRecord {
	createdAt := binding.CreationTimestamp.UTC().Format(TimestampFormat)
	updatedAt, _ := getTimeLastUpdatedTimestamp(&binding.ObjectMeta)
	return ServiceBindingRecord{
		GUID:                binding.Name,
		Type:                "app",
		Name:                binding.Spec.Name,
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
