package repositories

import (
	"context"
	"fmt"
	"slices"
	"time"

	"code.cloudfoundry.org/korifi/api/authorization"
	apierrors "code.cloudfoundry.org/korifi/api/errors"
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/model"
	"code.cloudfoundry.org/korifi/model/services"
	"code.cloudfoundry.org/korifi/tools"
	"github.com/BooleanCat/go-functional/iter"
	"github.com/google/uuid"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const ServiceBrokerResourceType = "Service Broker"

type CreateServiceBrokerMessage struct {
	Metadata    model.Metadata
	Broker      services.ServiceBroker
	Credentials services.BrokerCredentials
}

type ListServiceBrokerMessage struct {
	Names []string
}

func (l ListServiceBrokerMessage) matches(b korifiv1alpha1.CFServiceBroker) bool {
	if len(l.Names) == 0 {
		return true
	}

	return slices.Contains(l.Names, b.Spec.Name)
}

type ServiceBrokerRepo struct {
	userClientFactory authorization.UserK8sClientFactory
	rootNamespace     string
}

type ServiceBrokerResource struct {
	services.ServiceBroker
	model.CFResource
}

func NewServiceBrokerRepo(
	userClientFactory authorization.UserK8sClientFactory,
	rootNamespace string,
) *ServiceBrokerRepo {
	return &ServiceBrokerRepo{
		userClientFactory: userClientFactory,
		rootNamespace:     rootNamespace,
	}
}

func (r *ServiceBrokerRepo) CreateServiceBroker(ctx context.Context, authInfo authorization.Info, message CreateServiceBrokerMessage) (ServiceBrokerResource, error) {
	userClient, err := r.userClientFactory.BuildClient(authInfo)
	if err != nil {
		return ServiceBrokerResource{}, fmt.Errorf("failed to build user client: %w", err)
	}

	credsSecretData, err := tools.ToCredentialsSecretData(message.Credentials)
	if err != nil {
		return ServiceBrokerResource{}, fmt.Errorf("failed to create credentials secret data: %w", err)
	}

	credentialsSecretName := uuid.NewString()
	cfServiceBroker := &korifiv1alpha1.CFServiceBroker{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:   r.rootNamespace,
			Name:        uuid.NewString(),
			Labels:      message.Metadata.Labels,
			Annotations: message.Metadata.Annotations,
		},
		Spec: korifiv1alpha1.CFServiceBrokerSpec{
			ServiceBroker: message.Broker,
			Credentials: corev1.LocalObjectReference{
				Name: credentialsSecretName,
			},
		},
	}
	if err = userClient.Create(ctx, cfServiceBroker); err != nil {
		return ServiceBrokerResource{}, apierrors.FromK8sError(err, ServiceBrokerResourceType)
	}

	credentialsSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: r.rootNamespace,
			Name:      credentialsSecretName,
		},
		Data: credsSecretData,
	}
	err = controllerutil.SetOwnerReference(cfServiceBroker, credentialsSecret, scheme.Scheme)
	if err != nil {
		return ServiceBrokerResource{}, apierrors.FromK8sError(err, ServiceBrokerResourceType)
	}

	if err = userClient.Create(ctx, credentialsSecret); err != nil {
		return ServiceBrokerResource{}, apierrors.FromK8sError(err, ServiceBrokerResourceType)
	}

	return toServiceBrokerResource(*cfServiceBroker), nil
}

func toServiceBrokerResource(cfServiceBroker korifiv1alpha1.CFServiceBroker) ServiceBrokerResource {
	return ServiceBrokerResource{
		ServiceBroker: cfServiceBroker.Spec.ServiceBroker,
		CFResource: model.CFResource{
			GUID:      cfServiceBroker.Name,
			CreatedAt: cfServiceBroker.CreationTimestamp.Time,
			Metadata: model.Metadata{
				Labels:      cfServiceBroker.Labels,
				Annotations: cfServiceBroker.Annotations,
			},
		},
	}
}

func (r *ServiceBrokerRepo) GetState(ctx context.Context, authInfo authorization.Info, brokerGUID string) (model.CFResourceState, error) {
	userClient, err := r.userClientFactory.BuildClient(authInfo)
	if err != nil {
		return model.CFResourceState{}, fmt.Errorf("failed to build user client: %w", err)
	}

	cfServiceBroker := &korifiv1alpha1.CFServiceBroker{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: r.rootNamespace,
			Name:      brokerGUID,
		},
	}

	if err = userClient.Get(ctx, client.ObjectKeyFromObject(cfServiceBroker), cfServiceBroker); err != nil {
		return model.CFResourceState{}, apierrors.FromK8sError(err, ServiceBrokerResourceType)
	}

	if meta.IsStatusConditionTrue(cfServiceBroker.Status.Conditions, korifiv1alpha1.StatusConditionReady) {
		return model.CFResourceState{
			Status: model.CFResourceStatusReady,
		}, nil
	}

	return model.CFResourceState{}, nil
}

func (r *ServiceBrokerRepo) ListServiceBrokers(ctx context.Context, authInfo authorization.Info, message ListServiceBrokerMessage) ([]ServiceBrokerResource, error) {
	userClient, err := r.userClientFactory.BuildClient(authInfo)
	if err != nil {
		return nil, fmt.Errorf("failed to build user client: %w", err)
	}

	brokersList := &korifiv1alpha1.CFServiceBrokerList{}
	err = userClient.List(ctx, brokersList, client.InNamespace(r.rootNamespace))
	if err != nil {
		// All authenticated users are allowed to list brokers. Therefore, the
		// usual pattern of checking for forbidden error and return an empty
		// list does not make sense here
		return nil, fmt.Errorf("failed to list brokers: %w", apierrors.FromK8sError(err, ServiceBrokerResourceType))
	}

	brokers := iter.Lift(brokersList.Items).Filter(message.matches)
	return iter.Map(brokers, toServiceBrokerResource).Collect(), nil
}

func (r *ServiceBrokerRepo) GetServiceBroker(ctx context.Context, authInfo authorization.Info, guid string) (ServiceBrokerResource, error) {
	serviceBroker, err := r.getServiceBroker(ctx, authInfo, guid)
	if err != nil {
		return ServiceBrokerResource{}, err
	}
	return toServiceBrokerResource(*serviceBroker), nil
}

func (r *ServiceBrokerRepo) getServiceBroker(ctx context.Context, authInfo authorization.Info, guid string) (*korifiv1alpha1.CFServiceBroker, error) {
	userClient, err := r.userClientFactory.BuildClient(authInfo)
	if err != nil {
		return nil, fmt.Errorf("failed to build user client: %w", err)
	}

	serviceBroker := &korifiv1alpha1.CFServiceBroker{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: r.rootNamespace,
			Name:      guid,
		},
	}

	if err := userClient.Get(ctx, client.ObjectKeyFromObject(serviceBroker), serviceBroker); err != nil {
		return nil, apierrors.FromK8sError(err, ServiceBrokerResourceType)
	}

	return serviceBroker, nil
}

func (r *ServiceBrokerRepo) DeleteServiceBroker(ctx context.Context, authInfo authorization.Info, guid string) error {
	userClient, err := r.userClientFactory.BuildClient(authInfo)
	if err != nil {
		return fmt.Errorf("failed to build user client: %w", err)
	}

	serviceBroker := &korifiv1alpha1.CFServiceBroker{
		ObjectMeta: metav1.ObjectMeta{
			Name:      guid,
			Namespace: r.rootNamespace,
		},
	}

	return apierrors.FromK8sError(
		userClient.Delete(ctx, serviceBroker),
		ServiceBrokerResourceType,
	)
}

func (r *ServiceBrokerRepo) GetDeletedAt(ctx context.Context, authInfo authorization.Info, guid string) (*time.Time, error) {
	serviceBroker, err := r.getServiceBroker(ctx, authInfo, guid)
	if err != nil {
		return nil, err
	}

	return golangTime(serviceBroker.GetDeletionTimestamp()), nil
}
