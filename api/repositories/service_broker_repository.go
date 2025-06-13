package repositories

import (
	"context"
	"fmt"
	"slices"
	"time"

	"code.cloudfoundry.org/korifi/api/authorization"
	apierrors "code.cloudfoundry.org/korifi/api/errors"
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/tools"
	"github.com/BooleanCat/go-functional/v2/it"
	"github.com/google/uuid"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const ServiceBrokerResourceType = "Service Broker"

type BrokerCredentials struct {
	Username string
	Password string
}

type CreateServiceBrokerMessage struct {
	Name        string
	URL         string
	Credentials BrokerCredentials
	Metadata    Metadata
}

type ListServiceBrokerMessage struct {
	Names []string
	GUIDs []string
}

func (m *ListServiceBrokerMessage) toListOptions() []ListOption {
	return []ListOption{
		WithLabelIn(korifiv1alpha1.GUIDLabelKey, m.GUIDs),
		WithLabelIn(korifiv1alpha1.CFServiceBrokerDisplayNameLabelKey, tools.EncodeValuesToSha224(m.Names...)),
	}
}

type UpdateServiceBrokerMessage struct {
	GUID          string
	Name          *string
	URL           *string
	Credentials   *BrokerCredentials
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
	klient        Klient
	rootNamespace string
}

type ServiceBrokerRecord struct {
	GUID      string
	Name      string
	URL       string
	CreatedAt time.Time
	UpdatedAt *time.Time
	Metadata  Metadata
}

func (r ServiceBrokerRecord) Relationships() map[string]string {
	return nil
}

func NewServiceBrokerRepo(
	klient Klient,
	rootNamespace string,
) *ServiceBrokerRepo {
	return &ServiceBrokerRepo{
		klient:        klient,
		rootNamespace: rootNamespace,
	}
}

func (r *ServiceBrokerRepo) CreateServiceBroker(ctx context.Context, authInfo authorization.Info, message CreateServiceBrokerMessage) (ServiceBrokerRecord, error) {
	credsSecretData, err := tools.ToCredentialsSecretData(map[string]string{
		"username": message.Credentials.Username,
		"password": message.Credentials.Password,
	})
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
			Name: message.Name,
			URL:  message.URL,
			Credentials: corev1.LocalObjectReference{
				Name: credentialsSecretName,
			},
		},
	}
	if err = r.klient.Create(ctx, cfServiceBroker); err != nil {
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

	if err = r.klient.Create(ctx, credentialsSecret); err != nil {
		return ServiceBrokerRecord{}, apierrors.FromK8sError(err, ServiceBrokerResourceType)
	}

	return toServiceBrokerRecord(*cfServiceBroker), nil
}

func toServiceBrokerRecord(cfServiceBroker korifiv1alpha1.CFServiceBroker) ServiceBrokerRecord {
	return ServiceBrokerRecord{
		Name:      cfServiceBroker.Spec.Name,
		URL:       cfServiceBroker.Spec.URL,
		GUID:      cfServiceBroker.Name,
		CreatedAt: cfServiceBroker.CreationTimestamp.Time,
		Metadata: Metadata{
			Labels:      cfServiceBroker.Labels,
			Annotations: cfServiceBroker.Annotations,
		},
	}
}

func (r *ServiceBrokerRepo) GetState(ctx context.Context, authInfo authorization.Info, brokerGUID string) (ResourceState, error) {
	cfServiceBroker := &korifiv1alpha1.CFServiceBroker{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: r.rootNamespace,
			Name:      brokerGUID,
		},
	}

	if err := r.klient.Get(ctx, cfServiceBroker); err != nil {
		return ResourceStateUnknown, apierrors.FromK8sError(err, ServiceBrokerResourceType)
	}

	if cfServiceBroker.Generation != cfServiceBroker.Status.ObservedGeneration {
		return ResourceStateUnknown, nil
	}

	if meta.IsStatusConditionTrue(cfServiceBroker.Status.Conditions, korifiv1alpha1.StatusConditionReady) {
		return ResourceStateReady, nil
	}

	return ResourceStateUnknown, nil
}

func (r *ServiceBrokerRepo) ListServiceBrokers(ctx context.Context, authInfo authorization.Info, message ListServiceBrokerMessage) ([]ServiceBrokerRecord, error) {
	brokersList := &korifiv1alpha1.CFServiceBrokerList{}
	_, err := r.klient.List(ctx, brokersList, message.toListOptions()...)
	if err != nil {
		// All authenticated users are allowed to list brokers. Therefore, the
		// usual pattern of checking for forbidden error and return an empty
		// list does not make sense here
		return nil, fmt.Errorf("failed to list brokers: %w", apierrors.FromK8sError(err, ServiceBrokerResourceType))
	}

	return slices.Collect(it.Map(slices.Values(brokersList.Items), toServiceBrokerRecord)), nil
}

func (r *ServiceBrokerRepo) GetServiceBroker(ctx context.Context, authInfo authorization.Info, guid string) (ServiceBrokerRecord, error) {
	serviceBroker, err := r.getServiceBroker(ctx, authInfo, guid)
	if err != nil {
		return ServiceBrokerRecord{}, err
	}
	return toServiceBrokerRecord(*serviceBroker), nil
}

func (r *ServiceBrokerRepo) getServiceBroker(ctx context.Context, authInfo authorization.Info, guid string) (*korifiv1alpha1.CFServiceBroker, error) {
	serviceBroker := &korifiv1alpha1.CFServiceBroker{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: r.rootNamespace,
			Name:      guid,
		},
	}

	if err := r.klient.Get(ctx, serviceBroker); err != nil {
		return nil, apierrors.FromK8sError(err, ServiceBrokerResourceType)
	}

	return serviceBroker, nil
}

func (r *ServiceBrokerRepo) UpdateServiceBroker(ctx context.Context, authInfo authorization.Info, message UpdateServiceBrokerMessage) (ServiceBrokerRecord, error) {
	cfServiceBroker := &korifiv1alpha1.CFServiceBroker{
		ObjectMeta: metav1.ObjectMeta{
			Name:      message.GUID,
			Namespace: r.rootNamespace,
		},
	}

	if err := GetAndPatch(ctx, r.klient, cfServiceBroker, func() error {
		message.apply(cfServiceBroker)
		return nil
	}); err != nil {
		return ServiceBrokerRecord{}, apierrors.FromK8sError(err, ServiceBrokerResourceType)
	}

	if message.Credentials != nil {
		credsSecretData, err := tools.ToCredentialsSecretData(map[string]string{
			"username": message.Credentials.Username,
			"password": message.Credentials.Password,
		})
		if err != nil {
			return ServiceBrokerRecord{}, fmt.Errorf("failed to marshal credentials secret data for service broker: %w", err)
		}

		credentialsSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: r.rootNamespace,
				Name:      cfServiceBroker.Spec.Credentials.Name,
			},
		}

		if err := GetAndPatch(ctx, r.klient, credentialsSecret, func() error {
			credentialsSecret.Data = credsSecretData
			return nil
		}); err != nil {
			return ServiceBrokerRecord{}, apierrors.FromK8sError(err, ServiceBrokerResourceType)
		}
	}

	return toServiceBrokerRecord(*cfServiceBroker), nil
}

func (r *ServiceBrokerRepo) DeleteServiceBroker(ctx context.Context, authInfo authorization.Info, guid string) error {
	serviceBroker := &korifiv1alpha1.CFServiceBroker{
		ObjectMeta: metav1.ObjectMeta{
			Name:      guid,
			Namespace: r.rootNamespace,
		},
	}

	return apierrors.FromK8sError(
		r.klient.Delete(ctx, serviceBroker),
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
