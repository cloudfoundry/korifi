package repositories

import (
	"context"
	"fmt"

	"code.cloudfoundry.org/korifi/api/authorization"
	apierrors "code.cloudfoundry.org/korifi/api/errors"
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/controllers/services/credentials"
	"code.cloudfoundry.org/korifi/model"
	"code.cloudfoundry.org/korifi/model/services"
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

type ServiceBrokerRepo struct {
	userClientFactory authorization.UserK8sClientFactory
	rootNamespace     string
}

type ServiceBrokerResource struct {
	services.ServiceBroker
	model.Metadata
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

	credsSecretData, err := credentials.ToCredentialsSecretData(message.Credentials)
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

	return ServiceBrokerResource{
		ServiceBroker: cfServiceBroker.Spec.ServiceBroker,
		Metadata: model.Metadata{
			Labels:      cfServiceBroker.Labels,
			Annotations: cfServiceBroker.Annotations,
		},
		CFResource: model.CFResource{
			GUID:      cfServiceBroker.Name,
			CreatedAt: cfServiceBroker.CreationTimestamp.Time,
		},
	}, nil
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
