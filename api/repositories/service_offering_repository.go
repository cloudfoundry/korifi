package repositories

import (
	"context"
	"fmt"
	"time"

	"code.cloudfoundry.org/korifi/api/authorization"
	apierrors "code.cloudfoundry.org/korifi/api/errors"
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/tools"
	"github.com/BooleanCat/go-functional/v2/it"
	"github.com/BooleanCat/go-functional/v2/it/itx"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const ServiceOfferingResourceType = "Service Offering"

type ServiceOfferingRecord struct {
	Name              string
	GUID              string
	CreatedAt         time.Time
	UpdatedAt         *time.Time
	Metadata          Metadata
	Description       string
	Tags              []string
	Requires          []string
	DocumentationURL  *string
	BrokerCatalog     ServiceBrokerCatalog
	ServiceBrokerGUID string
}

type ServiceBrokerCatalog struct {
	ID       string
	Metadata map[string]any
	Features BrokerCatalogFeatures
}

type BrokerCatalogFeatures struct {
	PlanUpdateable       bool
	Bindable             bool
	InstancesRetrievable bool
	BindingsRetrievable  bool
	AllowContextUpdates  bool
}

func (r ServiceOfferingRecord) Relationships() map[string]string {
	return map[string]string{
		"service_broker": r.ServiceBrokerGUID,
	}
}

type ServiceOfferingRepo struct {
	klient               Klient
	rootNamespace        string
	brokerRepo           *ServiceBrokerRepo
	namespacePermissions *authorization.NamespacePermissions
}

type ListServiceOfferingMessage struct {
	Names       []string
	GUIDs       []string
	BrokerNames []string
}

type DeleteServiceOfferingMessage struct {
	GUID  string
	Purge bool
}

func (m *ListServiceOfferingMessage) matches(cfServiceOffering korifiv1alpha1.CFServiceOffering) bool {
	return tools.EmptyOrContains(m.Names, cfServiceOffering.Spec.Name) &&
		tools.EmptyOrContains(m.GUIDs, cfServiceOffering.Name) &&
		tools.EmptyOrContains(m.BrokerNames, cfServiceOffering.Labels[korifiv1alpha1.RelServiceBrokerNameLabel])
}

func NewServiceOfferingRepo(
	klient Klient,
	rootNamespace string,
	brokerRepo *ServiceBrokerRepo,
	namespacePermissions *authorization.NamespacePermissions,
) *ServiceOfferingRepo {
	return &ServiceOfferingRepo{
		klient:               klient,
		rootNamespace:        rootNamespace,
		brokerRepo:           brokerRepo,
		namespacePermissions: namespacePermissions,
	}
}

func (r *ServiceOfferingRepo) GetServiceOffering(ctx context.Context, authInfo authorization.Info, guid string) (ServiceOfferingRecord, error) {
	offering := &korifiv1alpha1.CFServiceOffering{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: r.rootNamespace,
			Name:      guid,
		},
	}

	if err := r.klient.Get(ctx, offering); err != nil {
		return ServiceOfferingRecord{}, fmt.Errorf("failed to get service offering: %s %w", guid, apierrors.FromK8sError(err, ServiceOfferingResourceType))
	}

	return offeringToRecord(*offering)
}

func (r *ServiceOfferingRepo) ListOfferings(ctx context.Context, authInfo authorization.Info, message ListServiceOfferingMessage) ([]ServiceOfferingRecord, error) {
	offeringsList := &korifiv1alpha1.CFServiceOfferingList{}
	err := r.klient.List(ctx, offeringsList, InNamespace(r.rootNamespace))
	if err != nil {
		if k8serrors.IsForbidden(err) {
			return []ServiceOfferingRecord{}, nil
		}

		return []ServiceOfferingRecord{}, fmt.Errorf("failed to list service offerings: %w",
			apierrors.FromK8sError(err, ServiceOfferingResourceType),
		)
	}

	return it.TryCollect(it.MapError(itx.FromSlice(offeringsList.Items).Filter(message.matches), offeringToRecord))
}

func (r *ServiceOfferingRepo) DeleteOffering(ctx context.Context, authInfo authorization.Info, message DeleteServiceOfferingMessage) error {
	offering := &korifiv1alpha1.CFServiceOffering{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: r.rootNamespace,
			Name:      message.GUID,
		},
	}

	if err := r.klient.Get(ctx, offering); err != nil {
		return fmt.Errorf("failed to get service offering: %w", apierrors.FromK8sError(err, ServiceOfferingResourceType))
	}

	if message.Purge {
		if err := r.purgeRelatedResources(ctx, authInfo, message.GUID); err != nil {
			return fmt.Errorf("failed to purge service offering resources: %w", apierrors.FromK8sError(err, ServiceOfferingResourceType))
		}
	}

	if err := r.klient.Delete(ctx, offering); err != nil {
		return fmt.Errorf("failed to delete service offering: %w", apierrors.FromK8sError(err, ServiceOfferingResourceType))
	}

	return nil
}

func offeringToRecord(offering korifiv1alpha1.CFServiceOffering) (ServiceOfferingRecord, error) {
	metadata, err := korifiv1alpha1.AsMap(offering.Spec.BrokerCatalog.Metadata)
	if err != nil {
		return ServiceOfferingRecord{}, err
	}

	return ServiceOfferingRecord{
		Name:             offering.Spec.Name,
		Description:      offering.Spec.Description,
		Tags:             offering.Spec.Tags,
		Requires:         offering.Spec.Requires,
		DocumentationURL: offering.Spec.DocumentationURL,
		BrokerCatalog: ServiceBrokerCatalog{
			ID:       offering.Spec.BrokerCatalog.ID,
			Metadata: metadata,
			Features: BrokerCatalogFeatures(offering.Spec.BrokerCatalog.Features),
		},
		GUID:      offering.Name,
		CreatedAt: offering.CreationTimestamp.Time,
		Metadata: Metadata{
			Labels:      offering.Labels,
			Annotations: offering.Annotations,
		},
		ServiceBrokerGUID: offering.Labels[korifiv1alpha1.RelServiceBrokerGUIDLabel],
	}, nil
}

func (r *ServiceOfferingRepo) purgeRelatedResources(ctx context.Context, authInfo authorization.Info, offeringGUID string) error {
	planGUIDs, err := r.deleteServicePlans(ctx, offeringGUID)
	if err != nil {
		return fmt.Errorf("failed to delete service plans: %w", apierrors.FromK8sError(err, ServicePlanResourceType))
	}

	authorizedSpaceNamespacesIter, err := authorizedSpaceNamespaces(ctx, authInfo, r.namespacePermissions)
	if err != nil {
		return fmt.Errorf("failed to list namespaces: %w", err)
	}

	serviceInstances, err := r.fetchServiceInstances(ctx, authorizedSpaceNamespacesIter, planGUIDs)
	if err != nil {
		return fmt.Errorf("failed to list service instances: %w", err)
	}

	for _, instance := range serviceInstances {
		err = r.klient.Patch(ctx, &instance, func() error {
			controllerutil.RemoveFinalizer(&instance, korifiv1alpha1.CFServiceInstanceFinalizerName)
			return nil
		})
		if err != nil {
			return fmt.Errorf("failed to remove finalizer for service instance: %s, %w", instance.Name, apierrors.FromK8sError(err, ServiceInstanceResourceType))
		}

		if err = r.klient.Delete(ctx, &instance); err != nil {
			return fmt.Errorf("failed to delete service instance: %w", apierrors.FromK8sError(err, ServiceInstanceResourceType))
		}

	}

	serviceBindings, err := r.fetchServiceBindings(ctx, authorizedSpaceNamespacesIter, planGUIDs)
	if err != nil {
		return fmt.Errorf("failed to list service bindings: %w", err)
	}

	for _, binding := range serviceBindings {
		err = r.klient.Patch(ctx, &binding, func() error {
			controllerutil.RemoveFinalizer(&binding, korifiv1alpha1.CFServiceBindingFinalizerName)
			return nil
		})
		if err != nil {
			return fmt.Errorf("failed to remove finalizer for service binding: %s, %w", binding.Name, apierrors.FromK8sError(err, ServiceBindingResourceType))
		}
	}

	return nil
}

func (r *ServiceOfferingRepo) deleteServicePlans(ctx context.Context, offeringGUID string) ([]string, error) {
	var planGUIDs []string
	plans := &korifiv1alpha1.CFServicePlanList{}

	if err := r.klient.List(ctx, plans, InNamespace(r.rootNamespace), WithLabel(korifiv1alpha1.RelServiceOfferingGUIDLabel, offeringGUID)); err != nil {
		return []string{}, fmt.Errorf("failed to list service plans: %w", err)
	}

	for _, plan := range plans.Items {
		planGUIDs = append(planGUIDs, plan.Name)
		if err := r.klient.Delete(ctx, &plan); err != nil {
			return []string{}, apierrors.FromK8sError(err, ServicePlanResourceType)
		}
	}

	return planGUIDs, nil
}

func (r *ServiceOfferingRepo) fetchServiceInstances(ctx context.Context, authorizedNamespaces itx.Iterator[string], planGUIDs []string) ([]korifiv1alpha1.CFServiceInstance, error) {
	var serviceInstances []korifiv1alpha1.CFServiceInstance

	for _, ns := range authorizedNamespaces.Collect() {
		instances := new(korifiv1alpha1.CFServiceInstanceList)

		err := r.klient.List(ctx, instances, InNamespace(ns))
		if err != nil {
			return []korifiv1alpha1.CFServiceInstance{}, fmt.Errorf("failed to list service instances: %w", err)
		}

		filtered := itx.FromSlice(instances.Items).Filter(func(serviceInstance korifiv1alpha1.CFServiceInstance) bool {
			return tools.EmptyOrContains(planGUIDs, serviceInstance.Spec.PlanGUID)
		}).Collect()

		serviceInstances = append(serviceInstances, filtered...)
	}

	return serviceInstances, nil
}

func (r *ServiceOfferingRepo) fetchServiceBindings(ctx context.Context, authorizedNamespaces itx.Iterator[string], planGUIDs []string) ([]korifiv1alpha1.CFServiceBinding, error) {
	var serviceBindings []korifiv1alpha1.CFServiceBinding

	for _, ns := range authorizedNamespaces.Collect() {
		bindings := new(korifiv1alpha1.CFServiceBindingList)

		err := r.klient.List(ctx, bindings, InNamespace(ns))
		if err != nil {
			return []korifiv1alpha1.CFServiceBinding{}, fmt.Errorf("failed to list service bindings: %w", err)
		}

		filtered := itx.FromSlice(bindings.Items).Filter(func(serviceBinding korifiv1alpha1.CFServiceBinding) bool {
			return tools.EmptyOrContains(planGUIDs, serviceBinding.Labels[korifiv1alpha1.PlanGUIDLabelKey])
		}).Collect()

		serviceBindings = append(serviceBindings, filtered...)
	}

	return serviceBindings, nil
}
