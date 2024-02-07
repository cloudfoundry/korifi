package repositories

import (
	"context"
	"fmt"
	"time"

	"code.cloudfoundry.org/korifi/api/authorization"
	apierrors "code.cloudfoundry.org/korifi/api/errors"
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/tools/k8s"

	"github.com/google/uuid"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	CFServiceBrokerGUIDLabel  = "korifi.cloudfoundry.org/service-broker-guid"
	ServiceBrokerResourceType = "Service Broker"
	CFNamespace               = "cf"
)

//type NamespaceGetter interface {
//	GetNamespaceForServiceBroker(ctx context.Context, guid string) (string, error)
//}

type ServiceBrokerRepo struct {
	namespaceRetriever   NamespaceRetriever
	userClientFactory    authorization.UserK8sClientFactory
	namespacePermissions *authorization.NamespacePermissions
}

func NewServiceBrokerRepo(
	namespaceRetriever NamespaceRetriever,
	userClientFactory authorization.UserK8sClientFactory,
	namespacePermissions *authorization.NamespacePermissions,
) *ServiceBrokerRepo {
	return &ServiceBrokerRepo{
		namespaceRetriever:   namespaceRetriever,
		userClientFactory:    userClientFactory,
		namespacePermissions: namespacePermissions,
	}
}

type CreateServiceBrokerMessage struct {
	Name        string
	URL         string
	Credentials map[string]string
	Labels      map[string]string
	Annotations map[string]string
}

type PatchServiceBrokerMessage struct {
	GUID        string
	Name        *string
	URL         *string
	Credentials *map[string]string
	MetadataPatch
}

func (p PatchServiceBrokerMessage) Apply(cfServiceBroker *korifiv1alpha1.CFServiceBroker) {
	if p.Name != nil {
		cfServiceBroker.Spec.Name = *p.Name
	}
	if p.URL != nil {
		cfServiceBroker.Spec.URL = *p.URL
	}
	p.MetadataPatch.Apply(cfServiceBroker)
}

type ListServiceBrokerMessage struct {
	Names         []string
	GUIDs         []string
	LabelSelector string
}

type DeleteServiceBrokerMessage struct {
	GUID string
}

type ServiceBrokerRecord struct {
	Name        string
	URL         string
	GUID        string
	SecretName  string
	Labels      map[string]string
	Annotations map[string]string
	CreatedAt   time.Time
	UpdatedAt   *time.Time
}

func (r *ServiceBrokerRepo) CreateServiceBroker(ctx context.Context, authInfo authorization.Info, message CreateServiceBrokerMessage) (ServiceBrokerRecord, error) {
	userClient, err := r.userClientFactory.BuildClient(authInfo)
	if err != nil {
		// untested
		return ServiceBrokerRecord{}, fmt.Errorf("failed to build user client: %w", err)
	}

	cfServiceBroker := message.toCFServiceBroker()
	if err != nil {
		return ServiceBrokerRecord{}, err
	}
	err = userClient.Create(ctx, &cfServiceBroker)
	if err != nil {
		return ServiceBrokerRecord{}, apierrors.FromK8sError(err, ServiceBrokerResourceType)
	}

	secretObj := cfServiceBrokerToSecret(cfServiceBroker)
	_, err = controllerutil.CreateOrPatch(ctx, userClient, secretObj, func() error {
		secretObj.StringData = message.Credentials
		return nil
	})
	if err != nil {
		return ServiceBrokerRecord{}, apierrors.FromK8sError(err, ServiceBrokerResourceType)
	}

	return cfServiceBrokerToServiceBrokerRecord(cfServiceBroker)
}

func (r *ServiceBrokerRepo) PatchServiceBroker(ctx context.Context, authInfo authorization.Info, message PatchServiceBrokerMessage) (ServiceBrokerRecord, error) {
	userClient, err := r.userClientFactory.BuildClient(authInfo)
	if err != nil {
		return ServiceBrokerRecord{}, fmt.Errorf("failed to build user client: %w", err)
	}

	var cfServiceBroker korifiv1alpha1.CFServiceBroker
	cfServiceBroker.Name = message.GUID
	cfServiceBroker.ObjectMeta.Namespace = CFNamespace
	if err = userClient.Get(ctx, client.ObjectKeyFromObject(&cfServiceBroker), &cfServiceBroker); err != nil {
		return ServiceBrokerRecord{}, apierrors.FromK8sError(err, ServiceBrokerResourceType)
	}

	err = k8s.PatchResource(ctx, userClient, &cfServiceBroker, func() {
		message.Apply(&cfServiceBroker)
	})
	if err != nil {
		return ServiceBrokerRecord{}, apierrors.FromK8sError(err, ServiceBrokerResourceType)
	}

	if message.Credentials != nil {
		secretObj := cfServiceBrokerToSecret(cfServiceBroker)
		_, err = controllerutil.CreateOrPatch(ctx, userClient, secretObj, func() error {
			secretObj.StringData = *message.Credentials
			return nil
		})
		if err != nil {
			return ServiceBrokerRecord{}, apierrors.FromK8sError(err, ServiceBrokerResourceType)
		}
	}

	return cfServiceBrokerToServiceBrokerRecord(cfServiceBroker)
}

// nolint:dupl
func (r *ServiceBrokerRepo) ListServiceBrokers(ctx context.Context, authInfo authorization.Info, message ListServiceBrokerMessage) ([]ServiceBrokerRecord, error) {

	userClient, err := r.userClientFactory.BuildClient(authInfo)
	if err != nil {
		return []ServiceBrokerRecord{}, fmt.Errorf("failed to build user client: %w", err)
	}

	preds := []func(korifiv1alpha1.CFServiceBroker) bool{
		SetPredicate(message.Names, func(b korifiv1alpha1.CFServiceBroker) string { return b.Spec.Name }),
		SetPredicate(message.GUIDs, func(b korifiv1alpha1.CFServiceBroker) string { return b.Name }),
	}

	labelSelector, err := labels.Parse(message.LabelSelector)
	if err != nil {
		return []ServiceBrokerRecord{}, apierrors.NewUnprocessableEntityError(err, "invalid label selector")
	}

	var filteredServiceBrokers []korifiv1alpha1.CFServiceBroker
	serviceBrokerList := new(korifiv1alpha1.CFServiceBrokerList)
	err = userClient.List(ctx, serviceBrokerList, client.InNamespace(CFNamespace), &client.ListOptions{LabelSelector: labelSelector})

	if err != nil {
		return []ServiceBrokerRecord{}, fmt.Errorf("failed to list service brokers in namespace %s: %w",
			CFNamespace,
			apierrors.FromK8sError(err, ServiceBrokerResourceType),
		)
	}
	filteredServiceBrokers = append(filteredServiceBrokers, Filter(serviceBrokerList.Items, preds...)...)

	return returnServiceBrokerList(filteredServiceBrokers)
}

func (r *ServiceBrokerRepo) GetServiceBroker(ctx context.Context, authInfo authorization.Info, guid string) (ServiceBrokerRecord, error) {
	userClient, err := r.userClientFactory.BuildClient(authInfo)
	if err != nil {
		return ServiceBrokerRecord{}, fmt.Errorf("failed to build user client: %w", err)
	}

	var serviceBroker korifiv1alpha1.CFServiceBroker
	if err := userClient.Get(ctx, client.ObjectKey{Namespace: CFNamespace, Name: guid}, &serviceBroker); err != nil {
		return ServiceBrokerRecord{}, fmt.Errorf("failed to get service broker: %w", apierrors.FromK8sError(err, ServiceBrokerResourceType))
	}

	return cfServiceBrokerToServiceBrokerRecord(serviceBroker)
}

func (r *ServiceBrokerRepo) DeleteServiceBroker(ctx context.Context, authInfo authorization.Info, message DeleteServiceBrokerMessage) error {
	userClient, err := r.userClientFactory.BuildClient(authInfo)
	if err != nil {
		return fmt.Errorf("failed to build user client: %w", err)
	}

	serviceBroker := &korifiv1alpha1.CFServiceBroker{
		ObjectMeta: metav1.ObjectMeta{
			Name:      message.GUID,
			Namespace: CFNamespace,
		},
	}

	if err := userClient.Delete(ctx, serviceBroker); err != nil {
		return fmt.Errorf("failed to delete service broker: %w", apierrors.FromK8sError(err, ServiceBrokerResourceType))
	}

	return nil
}

func (m CreateServiceBrokerMessage) toCFServiceBroker() korifiv1alpha1.CFServiceBroker {
	guid := uuid.NewString()

	broker := korifiv1alpha1.CFServiceBroker{
		ObjectMeta: metav1.ObjectMeta{
			Name:        guid,
			Namespace:   CFNamespace,
			Labels:      m.Labels,
			Annotations: m.Annotations,
		},
		Spec: korifiv1alpha1.CFServiceBrokerSpec{
			Name:       m.Name,
			URL:        m.URL,
			SecretName: guid,
		},
	}

	return broker
}

func cfServiceBrokerToServiceBrokerRecord(cfServiceBroker korifiv1alpha1.CFServiceBroker) (ServiceBrokerRecord, error) {
	return ServiceBrokerRecord{
		Name:        cfServiceBroker.Spec.Name,
		GUID:        cfServiceBroker.Name,
		SecretName:  cfServiceBroker.Spec.SecretName,
		URL:         cfServiceBroker.Spec.URL,
		Labels:      cfServiceBroker.Labels,
		Annotations: cfServiceBroker.Annotations,
		CreatedAt:   cfServiceBroker.CreationTimestamp.Time,
		UpdatedAt:   getLastUpdatedTime(&cfServiceBroker),
	}, nil
}

func cfServiceBrokerToSecret(cfServiceBroker korifiv1alpha1.CFServiceBroker) *corev1.Secret {
	labels := make(map[string]string, 1)
	labels[CFServiceBrokerGUIDLabel] = cfServiceBroker.Name

	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cfServiceBroker.Name,
			Namespace: cfServiceBroker.Namespace,
			Labels:    labels,
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: korifiv1alpha1.GroupVersion.String(),
					Kind:       "CFServiceBroker",
					Name:       cfServiceBroker.Name,
					UID:        cfServiceBroker.UID,
				},
			},
		},
	}
}

func returnServiceBrokerList(serviceBrokerList []korifiv1alpha1.CFServiceBroker) ([]ServiceBrokerRecord, error) {
	serviceBrokerRecords := make([]ServiceBrokerRecord, 0, len(serviceBrokerList))

	for _, serviceBroker := range serviceBrokerList {
		record, err := cfServiceBrokerToServiceBrokerRecord(serviceBroker)
		if err != nil {
			return nil, err
		}
		serviceBrokerRecords = append(serviceBrokerRecords, record)
	}
	return serviceBrokerRecords, nil
}
