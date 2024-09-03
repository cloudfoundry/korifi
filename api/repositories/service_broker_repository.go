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
	"github.com/BooleanCat/go-functional/v2/it"
	"github.com/BooleanCat/go-functional/v2/it/itx"
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
	GUIDs []string
}

func (l ListServiceBrokerMessage) matches(b korifiv1alpha1.CFServiceBroker) bool {
	return tools.EmptyOrContains(l.Names, b.Spec.Name) &&
		tools.EmptyOrContains(l.GUIDs, b.Name)
}

type UpdateServiceBrokerMessage struct {
	GUID          string
	Name          *string
	URL           *string
	Credentials   *services.BrokerCredentials
	MetadataPatch MetadataPatch
}

func (m UpdateServiceBrokerMessage) apply(broker *korifiv1alpha1.CFServiceBroker) {
	if m.Name != nil {
		broker.Spec.Name = *m.Name
	}

	if m.URL != nil {
		broker.Spec.URL = *m.URL
	}

	m.MetadataPatch.Apply(broker)
}

type ServiceBrokerRepo struct {
	userClientFactory authorization.UserK8sClientFactory
	rootNamespace     string
}

type ServiceBrokerRecord struct {
	services.ServiceBroker
	model.CFResource
}

func (r ServiceBrokerRecord) Relationships() map[string]string {
	return nil
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

func (r *ServiceBrokerRepo) CreateServiceBroker(ctx context.Context, authInfo authorization.Info, message CreateServiceBrokerMessage) (ServiceBrokerRecord, error) {
	userClient, err := r.userClientFactory.BuildClient(authInfo)
	if err != nil {
		return ServiceBrokerRecord{}, fmt.Errorf("failed to build user client: %w", err)
	}

	credsSecretData, err := tools.ToCredentialsSecretData(message.Credentials)
	if err != nil {
		return ServiceBrokerRecord{}, fmt.Errorf("failed to create credentials secret data: %w", err)
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
		return ServiceBrokerRecord{}, apierrors.FromK8sError(err, ServiceBrokerResourceType)
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
		return ServiceBrokerRecord{}, apierrors.FromK8sError(err, ServiceBrokerResourceType)
	}

	if err = userClient.Create(ctx, credentialsSecret); err != nil {
		return ServiceBrokerRecord{}, apierrors.FromK8sError(err, ServiceBrokerResourceType)
	}

	return toServiceBrokerRecord(*cfServiceBroker), nil
}

func toServiceBrokerRecord(cfServiceBroker korifiv1alpha1.CFServiceBroker) ServiceBrokerRecord {
	return ServiceBrokerRecord{
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
		return model.CFResourceStateUnknown, fmt.Errorf("failed to build user client: %w", err)
	}

	cfServiceBroker := &korifiv1alpha1.CFServiceBroker{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: r.rootNamespace,
			Name:      brokerGUID,
		},
	}

	if err = userClient.Get(ctx, client.ObjectKeyFromObject(cfServiceBroker), cfServiceBroker); err != nil {
		return model.CFResourceStateUnknown, apierrors.FromK8sError(err, ServiceBrokerResourceType)
	}

	if cfServiceBroker.Generation != cfServiceBroker.Status.ObservedGeneration {
		return model.CFResourceStateUnknown, nil
	}

	if meta.IsStatusConditionTrue(cfServiceBroker.Status.Conditions, korifiv1alpha1.StatusConditionReady) {
		return model.CFResourceStateReady, nil
	}

	return model.CFResourceStateUnknown, nil
}

func (r *ServiceBrokerRepo) ListServiceBrokers(ctx context.Context, authInfo authorization.Info, message ListServiceBrokerMessage) ([]ServiceBrokerRecord, error) {
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

	brokers := itx.FromSlice(brokersList.Items).Filter(message.matches)

	return slices.Collect(it.Map(brokers, toServiceBrokerRecord)), nil
}

func (r *ServiceBrokerRepo) GetServiceBroker(ctx context.Context, authInfo authorization.Info, guid string) (ServiceBrokerRecord, error) {
	serviceBroker, err := r.getServiceBroker(ctx, authInfo, guid)
	if err != nil {
		return ServiceBrokerRecord{}, err
	}
	return toServiceBrokerRecord(*serviceBroker), nil
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

func (r *ServiceBrokerRepo) UpdateServiceBroker(ctx context.Context, authInfo authorization.Info, message UpdateServiceBrokerMessage) (ServiceBrokerRecord, error) {
	userClient, err := r.userClientFactory.BuildClient(authInfo)
	if err != nil {
		return ServiceBrokerRecord{}, fmt.Errorf("failed to build user client: %w", err)
	}

	cfServiceBroker := &korifiv1alpha1.CFServiceBroker{
		ObjectMeta: metav1.ObjectMeta{
			Name:      message.GUID,
			Namespace: r.rootNamespace,
		},
	}

	if err = PatchResource(ctx, userClient, cfServiceBroker, func() {
		message.apply(cfServiceBroker)
	}); err != nil {
		return ServiceBrokerRecord{}, apierrors.FromK8sError(err, ServiceBrokerResourceType)
	}

	if message.Credentials != nil {
		credsSecretData, err := tools.ToCredentialsSecretData(message.Credentials)
		if err != nil {
			return ServiceBrokerRecord{}, fmt.Errorf("failed to marshal credentials secret data for service broker: %w", err)
		}

		credentialsSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: r.rootNamespace,
				Name:      cfServiceBroker.Spec.Credentials.Name,
			},
		}

		if err := PatchResource(ctx, userClient, credentialsSecret, func() {
			credentialsSecret.Data = credsSecretData
		}); err != nil {
			return ServiceBrokerRecord{}, apierrors.FromK8sError(err, ServiceBrokerResourceType)
		}
	}

	return toServiceBrokerRecord(*cfServiceBroker), nil
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
