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
	rootNSKlient      Klient
	spaceScopedKlient Klient
	rootNamespace     string
}

type ListServiceOfferingMessage struct {
	Names       []string
	GUIDs       []string
	BrokerNames []string
	OrderBy     string
	Pagination  Pagination
}

type UpdateServiceOfferingMessage struct {
	GUID          string
	MetadataPatch MetadataPatch
}

type DeleteServiceOfferingMessage struct {
	GUID  string
	Purge bool
}

func (m UpdateServiceOfferingMessage) apply(offering *korifiv1alpha1.CFServiceOffering) {
	m.MetadataPatch.Apply(offering)
}

func (m *ListServiceOfferingMessage) toListOptions() []ListOption {
	return []ListOption{
		WithLabelIn(korifiv1alpha1.CFServiceOfferingNameKey, tools.EncodeValuesToSha224(m.Names...)),
		WithLabelIn(korifiv1alpha1.GUIDLabelKey, m.GUIDs),
		WithLabelIn(korifiv1alpha1.RelServiceBrokerNameLabel, tools.EncodeValuesToSha224(m.BrokerNames...)),
		WithOrdering(m.OrderBy,
			"name", "Display Name",
		),
		WithPaging(m.Pagination),
	}
}

func NewServiceOfferingRepo(
	rootNSKlient Klient,
	spaceScopedKlient Klient,
	rootNamespace string,
) *ServiceOfferingRepo {
	return &ServiceOfferingRepo{
		rootNSKlient:      rootNSKlient,
		spaceScopedKlient: spaceScopedKlient,
		rootNamespace:     rootNamespace,
	}
}

func (r *ServiceOfferingRepo) GetServiceOffering(ctx context.Context, authInfo authorization.Info, guid string) (ServiceOfferingRecord, error) {
	offering := &korifiv1alpha1.CFServiceOffering{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: r.rootNamespace,
			Name:      guid,
		},
	}

	if err := r.rootNSKlient.Get(ctx, offering); err != nil {
		return ServiceOfferingRecord{}, fmt.Errorf("failed to get service offering: %s %w", guid, apierrors.FromK8sError(err, ServiceOfferingResourceType))
	}

	return offeringToRecord(*offering)
}

func (r *ServiceOfferingRepo) ListOfferings(ctx context.Context, authInfo authorization.Info, message ListServiceOfferingMessage) (ListResult[ServiceOfferingRecord], error) {
	offeringsList := &korifiv1alpha1.CFServiceOfferingList{}
	pageInfo, err := r.rootNSKlient.List(ctx, offeringsList, message.toListOptions()...)
	if err != nil {
		if k8serrors.IsForbidden(err) {
			return ListResult[ServiceOfferingRecord]{}, nil
		}

		return ListResult[ServiceOfferingRecord]{}, fmt.Errorf("failed to list service offerings: %w",
			apierrors.FromK8sError(err, ServiceOfferingResourceType),
		)
	}

	records, err := it.TryCollect(it.MapError(itx.FromSlice(offeringsList.Items), offeringToRecord))
	if err != nil {
		return ListResult[ServiceOfferingRecord]{}, fmt.Errorf("failed to convert service offerings to records: %w", err)
	}
	return ListResult[ServiceOfferingRecord]{
		PageInfo: pageInfo,
		Records:  records,
	}, nil
}

func (r *ServiceOfferingRepo) DeleteOffering(ctx context.Context, authInfo authorization.Info, message DeleteServiceOfferingMessage) error {
	offering := &korifiv1alpha1.CFServiceOffering{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: r.rootNamespace,
			Name:      message.GUID,
		},
	}

	if err := r.rootNSKlient.Get(ctx, offering); err != nil {
		return fmt.Errorf("failed to get service offering: %w", apierrors.FromK8sError(err, ServiceOfferingResourceType))
	}

	if message.Purge {
		if err := r.purgeRelatedResources(ctx, message.GUID); err != nil {
			return fmt.Errorf("failed to purge service offering resources: %w", apierrors.FromK8sError(err, ServiceOfferingResourceType))
		}
	}

	if err := r.rootNSKlient.Delete(ctx, offering); err != nil {
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

func (r *ServiceOfferingRepo) purgeRelatedResources(ctx context.Context, offeringGUID string) error {
	planGUIDs, err := r.deleteServicePlans(ctx, offeringGUID)
	if err != nil {
		return fmt.Errorf("failed to delete service plans: %w", apierrors.FromK8sError(err, ServicePlanResourceType))
	}

	serviceInstances, err := r.fetchServiceInstances(ctx, planGUIDs)
	if err != nil {
		return fmt.Errorf("failed to list service instances: %w", err)
	}

	serviceInstanceGUIDs := slices.Collect(it.Map(slices.Values(serviceInstances), func(instance korifiv1alpha1.CFServiceInstance) string {
		return instance.Name
	}))

	serviceBindings, err := r.fetchServiceBindings(ctx, serviceInstanceGUIDs)
	if err != nil {
		return fmt.Errorf("failed to list service bindings: %w", err)
	}

	for _, binding := range serviceBindings {
		err = r.spaceScopedKlient.Patch(ctx, &binding, func() error {
			controllerutil.RemoveFinalizer(&binding, korifiv1alpha1.CFServiceBindingFinalizerName)
			return nil
		})
		if err != nil {
			return fmt.Errorf("failed to remove finalizer for service binding: %s, %w", binding.Name, apierrors.FromK8sError(err, ServiceBindingResourceType))
		}
	}

	for _, instance := range serviceInstances {
		err = r.spaceScopedKlient.Patch(ctx, &instance, func() error {
			controllerutil.RemoveFinalizer(&instance, korifiv1alpha1.CFServiceInstanceFinalizerName)
			return nil
		})
		if err != nil {
			return fmt.Errorf("failed to remove finalizer for service instance: %s, %w", instance.Name, apierrors.FromK8sError(err, ServiceInstanceResourceType))
		}

		if err = r.spaceScopedKlient.Delete(ctx, &instance); err != nil {
			return fmt.Errorf("failed to delete service instance: %w", apierrors.FromK8sError(err, ServiceInstanceResourceType))
		}

	}

	return nil
}

func (r *ServiceOfferingRepo) deleteServicePlans(ctx context.Context, offeringGUID string) ([]string, error) {
	var planGUIDs []string
	plans := &korifiv1alpha1.CFServicePlanList{}

	if _, err := r.rootNSKlient.List(ctx, plans, WithLabel(korifiv1alpha1.RelServiceOfferingGUIDLabel, offeringGUID)); err != nil {
		return []string{}, fmt.Errorf("failed to list service plans: %w", err)
	}

	for _, plan := range plans.Items {
		planGUIDs = append(planGUIDs, plan.Name)
		if err := r.rootNSKlient.Delete(ctx, &plan); err != nil {
			return []string{}, apierrors.FromK8sError(err, ServicePlanResourceType)
		}
	}

	return planGUIDs, nil
}

func (r *ServiceOfferingRepo) fetchServiceInstances(ctx context.Context, planGUIDs []string) ([]korifiv1alpha1.CFServiceInstance, error) {
	instances := new(korifiv1alpha1.CFServiceInstanceList)
	_, err := r.spaceScopedKlient.List(ctx, instances, WithLabelIn(korifiv1alpha1.PlanGUIDLabelKey, planGUIDs))
	if err != nil {
		return []korifiv1alpha1.CFServiceInstance{}, fmt.Errorf("failed to list service instances: %w", err)
	}

	return instances.Items, nil
}

func (r *ServiceOfferingRepo) fetchServiceBindings(ctx context.Context, serviceInstanceGUIDs []string) ([]korifiv1alpha1.CFServiceBinding, error) {
	bindings := new(korifiv1alpha1.CFServiceBindingList)
	_, err := r.spaceScopedKlient.List(ctx, bindings, WithLabelStrictlyIn(korifiv1alpha1.CFServiceInstanceGUIDLabelKey, serviceInstanceGUIDs))
	if err != nil {
		return []korifiv1alpha1.CFServiceBinding{}, fmt.Errorf("failed to list service bindings: %w", err)
	}

	return bindings.Items, nil
}

func (r *ServiceOfferingRepo) UpdateServiceOffering(ctx context.Context, authInfo authorization.Info, message UpdateServiceOfferingMessage) (ServiceOfferingRecord, error) {
	offering := &korifiv1alpha1.CFServiceOffering{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: r.rootNamespace,
			Name:      message.GUID,
		},
	}
	if err := GetAndPatch(ctx, r.rootNSKlient, offering, func() error {
		message.apply(offering)
		return nil
	}); err != nil {
		return ServiceOfferingRecord{}, fmt.Errorf("failed to patch service offering metadata: %w", apierrors.FromK8sError(err, ServiceOfferingResourceType))
	}

	return offeringToRecord(*offering)
}
