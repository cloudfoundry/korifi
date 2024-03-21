package repositories

import (
	"context"
	"fmt"

	"code.cloudfoundry.org/korifi/api/authorization"
	apierrors "code.cloudfoundry.org/korifi/api/errors"
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/tools/k8s"

	"github.com/google/uuid"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	CFServiceBrokerGUIDLabel  = "korifi.cloudfoundry.org/service-broker-guid"
	ServiceBrokerResourceType = "Service Broker"
)

type ServiceBrokerRepo struct {
	namespaceRetriever   NamespaceRetriever
	userClientFactory    authorization.UserK8sClientFactory
	namespacePermissions *authorization.NamespacePermissions
	rootNamespace        string
}

func NewServiceBrokerRepo(
	namespaceRetriever NamespaceRetriever,
	userClientFactory authorization.UserK8sClientFactory,
	namespacePermissions *authorization.NamespacePermissions,
	rootNamespace string,
) *ServiceBrokerRepo {
	return &ServiceBrokerRepo{
		namespaceRetriever:   namespaceRetriever,
		userClientFactory:    userClientFactory,
		namespacePermissions: namespacePermissions,
		rootNamespace:        rootNamespace,
	}
}

type ListServiceBrokerMessage struct {
	Names         []string
	GUIDs         []string
	LabelSelector string
}

func toServiceBrokerResource(cfServiceBroker *korifiv1alpha1.CFServiceBroker) korifiv1alpha1.ServiceBrokerResource {
	return korifiv1alpha1.ServiceBrokerResource{
		ServiceBroker: cfServiceBroker.Spec.ServiceBroker,
		CFResource: korifiv1alpha1.CFResource{
			GUID:  cfServiceBroker.Name,
			Ready: meta.IsStatusConditionTrue(cfServiceBroker.Status.Conditions, "Ready"),
		},
	}
}

func (r *ServiceBrokerRepo) CreateServiceBroker(ctx context.Context, authInfo authorization.Info, serviceBroker korifiv1alpha1.ServiceBroker, brokerAuth korifiv1alpha1.BasicAuthentication) (korifiv1alpha1.ServiceBrokerResource, error) {
	userClient, err := r.userClientFactory.BuildClient(authInfo)
	if err != nil {
		// untested
		return korifiv1alpha1.ServiceBrokerResource{}, fmt.Errorf("failed to build user client: %w", err)
	}

	cfServiceBroker := r.newCFServiceBroker(serviceBroker)
	err = userClient.Create(ctx, cfServiceBroker)
	if err != nil {
		return korifiv1alpha1.ServiceBrokerResource{}, apierrors.FromK8sError(err, ServiceBrokerResourceType)
	}

	secretObj := cfServiceBrokerToSecret(cfServiceBroker)
	_, err = controllerutil.CreateOrPatch(ctx, userClient, secretObj, func() error {
		secretObj.StringData = map[string]string{
			"username": brokerAuth.Credentials.Username,
			"password": brokerAuth.Credentials.Password,
		}
		return nil
	})

	if err != nil {
		return korifiv1alpha1.ServiceBrokerResource{}, apierrors.FromK8sError(err, ServiceBrokerResourceType)
	}

	return toServiceBrokerResource(cfServiceBroker), nil
}

func (r *ServiceBrokerRepo) PatchServiceBroker(ctx context.Context, authInfo authorization.Info, guid string, serviceBrokerPatch korifiv1alpha1.ServiceBrokerPatch, brokerAuth *korifiv1alpha1.BasicAuthentication) (korifiv1alpha1.ServiceBrokerResource, error) {
	userClient, err := r.userClientFactory.BuildClient(authInfo)
	if err != nil {
		return korifiv1alpha1.ServiceBrokerResource{}, fmt.Errorf("failed to build user client: %w", err)
	}

	cfServiceBroker := &korifiv1alpha1.CFServiceBroker{
		ObjectMeta: metav1.ObjectMeta{
			Name:      guid,
			Namespace: r.rootNamespace,
		},
	}

	if err = userClient.Get(ctx, client.ObjectKeyFromObject(cfServiceBroker), cfServiceBroker); err != nil {
		return korifiv1alpha1.ServiceBrokerResource{}, apierrors.FromK8sError(err, ServiceBrokerResourceType)
	}

	err = k8s.PatchResource(ctx, userClient, cfServiceBroker, func() {
		cfServiceBroker.Spec.ServiceBroker.Patch(serviceBrokerPatch)
	})
	if err != nil {
		return korifiv1alpha1.ServiceBrokerResource{}, apierrors.FromK8sError(err, ServiceBrokerResourceType)
	}

	if brokerAuth != nil {
		secretObj := cfServiceBrokerToSecret(cfServiceBroker)
		_, err = controllerutil.CreateOrPatch(ctx, userClient, secretObj, func() error {
			secretObj.StringData = map[string]string{
				"username": brokerAuth.Credentials.Username,
				"password": brokerAuth.Credentials.Password,
			}
			return nil
		})
		if err != nil {
			return korifiv1alpha1.ServiceBrokerResource{}, apierrors.FromK8sError(err, ServiceBrokerResourceType)
		}
	}

	return toServiceBrokerResource(cfServiceBroker), nil
}

// nolint:dupl
func (r *ServiceBrokerRepo) ListServiceBrokers(ctx context.Context, authInfo authorization.Info, message ListServiceBrokerMessage) ([]korifiv1alpha1.ServiceBrokerResource, error) {
	userClient, err := r.userClientFactory.BuildClient(authInfo)
	if err != nil {
		return []korifiv1alpha1.ServiceBrokerResource{}, fmt.Errorf("failed to build user client: %w", err)
	}

	preds := []func(korifiv1alpha1.CFServiceBroker) bool{
		SetPredicate(message.Names, func(b korifiv1alpha1.CFServiceBroker) string { return b.Spec.Name }),
		SetPredicate(message.GUIDs, func(b korifiv1alpha1.CFServiceBroker) string { return b.Name }),
	}

	labelSelector, err := labels.Parse(message.LabelSelector)
	if err != nil {
		return []korifiv1alpha1.ServiceBrokerResource{}, apierrors.NewUnprocessableEntityError(err, "invalid label selector")
	}

	var filteredServiceBrokers []korifiv1alpha1.CFServiceBroker
	serviceBrokerList := new(korifiv1alpha1.CFServiceBrokerList)
	err = userClient.List(ctx, serviceBrokerList, client.InNamespace(r.rootNamespace), &client.ListOptions{LabelSelector: labelSelector})

	if err != nil {
		return []korifiv1alpha1.ServiceBrokerResource{}, fmt.Errorf("failed to list service brokers in namespace %s: %w",
			r.rootNamespace,

			apierrors.FromK8sError(err, ServiceBrokerResourceType),
		)
	}
	filteredServiceBrokers = append(filteredServiceBrokers, Filter(serviceBrokerList.Items, preds...)...)

	return returnServiceBrokerList(filteredServiceBrokers), nil
}

func (r *ServiceBrokerRepo) GetServiceBroker(ctx context.Context, authInfo authorization.Info, guid string) (korifiv1alpha1.ServiceBrokerResource, error) {
	userClient, err := r.userClientFactory.BuildClient(authInfo)
	if err != nil {
		return korifiv1alpha1.ServiceBrokerResource{}, fmt.Errorf("failed to build user client: %w", err)
	}

	var serviceBroker korifiv1alpha1.CFServiceBroker

	if err := userClient.Get(ctx, client.ObjectKey{Namespace: r.rootNamespace, Name: guid}, &serviceBroker); err != nil {
		return korifiv1alpha1.ServiceBrokerResource{}, fmt.Errorf("failed to get service broker: %w", apierrors.FromK8sError(err, ServiceBrokerResourceType))
	}

	return toServiceBrokerResource(&serviceBroker), nil
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

	if err := userClient.Delete(ctx, serviceBroker); err != nil {
		return fmt.Errorf("failed to delete service broker: %w", apierrors.FromK8sError(err, ServiceBrokerResourceType))
	}

	return nil
}

func (r *ServiceBrokerRepo) newCFServiceBroker(serviceBroker korifiv1alpha1.ServiceBroker) *korifiv1alpha1.CFServiceBroker {
	meta := metav1.ObjectMeta{
		Name:      uuid.NewString(),
		Namespace: r.rootNamespace,
	}
	if serviceBroker.Metadata != nil {
		meta.Annotations = serviceBroker.Metadata.Annotations
		meta.Labels = serviceBroker.Metadata.Labels
	}
	return &korifiv1alpha1.CFServiceBroker{
		ObjectMeta: meta,
		Spec: korifiv1alpha1.CFServiceBrokerSpec{
			ServiceBroker: serviceBroker,
			SecretName:    meta.Name,
		},
	}
}

func cfServiceBrokerToSecret(cfServiceBroker *korifiv1alpha1.CFServiceBroker) *corev1.Secret {
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

func returnServiceBrokerList(serviceBrokerList []korifiv1alpha1.CFServiceBroker) []korifiv1alpha1.ServiceBrokerResource {
	serviceBrokerResources := make([]korifiv1alpha1.ServiceBrokerResource, 0, len(serviceBrokerList))

	for _, serviceBroker := range serviceBrokerList {
		serviceBrokerResources = append(serviceBrokerResources, toServiceBrokerResource(&serviceBroker))
	}
	return serviceBrokerResources
}
