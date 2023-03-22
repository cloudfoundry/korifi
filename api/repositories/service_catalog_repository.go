package repositories

import (
	"context"
	"fmt"

	"code.cloudfoundry.org/korifi/api/authorization"
	trinityv1alpha1 "github.tools.sap/neoCoreArchitecture/trinity-service-manager/controllers/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

type ServiceCatalogRepo struct {
	rootNamespace     string
	userClientFactory authorization.UserK8sClientFactory
}

func NewServiceCatalogRepo(
	userClientFactory authorization.UserK8sClientFactory,
	rootNamespace string,
) *ServiceCatalogRepo {
	return &ServiceCatalogRepo{
		rootNamespace:     rootNamespace,
		userClientFactory: userClientFactory,
	}
}

type ListServiceOfferingMessage struct {
	Names []string
}

type ListServicePlanMessage struct {
	Names                []string
	ServiceOfferingNames []string
	ServiceOfferingGUIDs []string
}

type ServiceOfferingRecord struct {
	GUID                 string
	Name                 string
	Description          string
	Available            bool
	Tags                 []string
	Requires             []string
	CreatedAt            string
	UpdatedAt            string
	Shareable            bool
	DocumentationUrl     string
	BrokerId             string
	Bindable             bool
	PlanUpdateable       bool
	InstancesRetrievable bool
	BindingsRetrievable  bool
	AllowContextUpdates  bool
	CatalogId            string
}

type ServicePlanRecord struct {
	GUID                string
	Name                string
	Description         string
	Available           bool
	CreatedAt           string
	UpdatedAt           string
	VisibilityType      string
	Free                bool
	Costs               []struct{}
	MaintenanceInfo     struct{}
	BrokerCatalog       struct{}
	ServiceOfferingGUID string
	BrokerId            string
	Bindable            bool
	PlanUpdateable      bool
	CatalogId           string
	Schemas             map[string]any
}

type ServiceInstanceSchema struct {
	Create ServiceInstanceSchemaCreate `json:"create"`
	Update ServiceInstanceSchemaUpdate `json:"update"`
}

type ServiceInstanceSchemaCreate struct {
	Parameters SchemaParameters `json:"parameters"`
}

type ServiceInstanceSchemaUpdate struct {
	Parameters SchemaParameters `json:"parameters"`
}

type ServiceBindingSchema struct {
	Create ServiceBindingSchemaCreate `json:"create"`
}

type ServiceBindingSchemaCreate struct {
	Parameters SchemaParameters `json:"parameters"`
}

type SchemaParameters map[string]any

func (r *ServiceCatalogRepo) ListServiceOfferings(ctx context.Context, authInfo authorization.Info, message ListServiceOfferingMessage) ([]ServiceOfferingRecord, error) {
	userClient, err := r.userClientFactory.BuildClient(authInfo)
	if err != nil {
		return []ServiceOfferingRecord{}, fmt.Errorf("failed to build user client: %w", err)
	}

	var result []ServiceOfferingRecord

	allServiceOferings := &trinityv1alpha1.CFServiceOfferingList{}
	err = userClient.List(ctx, allServiceOferings, client.InNamespace(r.rootNamespace))
	if err != nil {
		return []ServiceOfferingRecord{}, fmt.Errorf("failed to list service offerings: %w", err)
	}

	for _, o := range allServiceOferings.Items {
		if !filterAppliesTo(o.Spec.OfferingName, message.Names) {
			continue
		}

		result = append(result, serviceOfferingToRecord(&o))

	}

	return result, nil
}

func (r *ServiceCatalogRepo) GetServiceOffering(ctx context.Context, authInfo authorization.Info, guid string) (ServiceOfferingRecord, error) {
	userClient, err := r.userClientFactory.BuildClient(authInfo)
	if err != nil {
		return ServiceOfferingRecord{}, fmt.Errorf("failed to build user client: %w", err)
	}

	serviceOffering := &trinityv1alpha1.CFServiceOffering{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: r.rootNamespace,
			Name:      guid,
		},
	}
	err = userClient.Get(ctx, client.ObjectKeyFromObject(serviceOffering), serviceOffering)
	if err != nil {
		return ServiceOfferingRecord{}, fmt.Errorf("failed to get service offering: %w", err)
	}

	return serviceOfferingToRecord(serviceOffering), nil
}

func (r *ServiceCatalogRepo) ListServicePlans(ctx context.Context, authInfo authorization.Info, message ListServicePlanMessage) ([]ServicePlanRecord, error) {
	userClient, err := r.userClientFactory.BuildClient(authInfo)
	if err != nil {
		return []ServicePlanRecord{}, fmt.Errorf("failed to build user client: %w", err)
	}

	var result []ServicePlanRecord

	allServicePlans := &trinityv1alpha1.CFServicePlanList{}
	err = userClient.List(ctx, allServicePlans, client.InNamespace(r.rootNamespace))
	if err != nil {
		return []ServicePlanRecord{}, fmt.Errorf("failed to list service plans: %w", err)
	}

	offeringGuids, err := r.getOfferingGuids(ctx, userClient, message.ServiceOfferingNames)
	if err != nil {
		return []ServicePlanRecord{}, fmt.Errorf("failed to list service offerings: %w", err)
	}
	offeringGuids = append(offeringGuids, message.ServiceOfferingGUIDs...)

	for _, p := range allServicePlans.Items {
		if !filterAppliesTo(p.Spec.PlanName, message.Names) {
			continue
		}

		if !filterAppliesTo(p.Spec.Relationships.ServiceOfferingGUID, offeringGuids) {
			continue
		}

		result = append(result, servicePlanToRecord(&p))

	}

	return result, nil
}

func (r *ServiceCatalogRepo) GetServicePlan(ctx context.Context, authInfo authorization.Info, guid string) (ServicePlanRecord, error) {
	userClient, err := r.userClientFactory.BuildClient(authInfo)
	if err != nil {
		return ServicePlanRecord{}, fmt.Errorf("failed to build user client: %w", err)
	}

	servicePlan := &trinityv1alpha1.CFServicePlan{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: r.rootNamespace,
			Name:      guid,
		},
	}
	err = userClient.Get(ctx, client.ObjectKeyFromObject(servicePlan), servicePlan)
	if err != nil {
		return ServicePlanRecord{}, fmt.Errorf("failed to get service plan: %w", err)
	}

	return servicePlanToRecord(servicePlan), nil
}

func (r *ServiceCatalogRepo) getOfferingGuids(ctx context.Context, userClient client.Client, names []string) ([]string, error) {
	offerings := &trinityv1alpha1.CFServiceOfferingList{}
	err := userClient.List(ctx, offerings, client.InNamespace(r.rootNamespace))
	if err != nil {
		return nil, err
	}

	guids := []string{}
	for _, o := range offerings.Items {
		if !filterAppliesTo(o.Spec.OfferingName, names) {
			continue
		}
		guids = append(guids, o.Spec.GUID)

	}

	return guids, nil
}

func filterAppliesTo(s string, filter []string) bool {
	if len(filter) == 0 {
		return true
	}

	for _, f := range filter {
		if s == f {
			return true
		}
	}

	return false
}

func servicePlanToRecord(servicePlan *trinityv1alpha1.CFServicePlan) ServicePlanRecord {
	return ServicePlanRecord{
		GUID:                servicePlan.Spec.GUID,
		Name:                servicePlan.Spec.PlanName,
		Description:         servicePlan.Spec.Description,
		Available:           servicePlan.Spec.Available,
		CreatedAt:           servicePlan.Spec.CreationTimestamp.UTC().Format(TimestampFormat),
		UpdatedAt:           servicePlan.Spec.UpdatedTimestamp.UTC().Format(TimestampFormat),
		VisibilityType:      "public",
		Free:                servicePlan.Spec.Free,
		ServiceOfferingGUID: servicePlan.Spec.Relationships.ServiceOfferingGUID,
		Bindable:            servicePlan.Spec.Bindable,
		PlanUpdateable:      servicePlan.Spec.PlanUpdateable,
		CatalogId:           servicePlan.Spec.CatalogId,
		Schemas: map[string]any{
			"service_instance": ServiceInstanceSchema{
				Create: ServiceInstanceSchemaCreate{
					Parameters: map[string]any{},
				},
				Update: ServiceInstanceSchemaUpdate{
					Parameters: map[string]any{},
				},
			},
			"service_binding": ServiceBindingSchema{
				Create: ServiceBindingSchemaCreate{
					Parameters: map[string]any{},
				},
			},
		},
	}
}

func serviceOfferingToRecord(offering *trinityv1alpha1.CFServiceOffering) ServiceOfferingRecord {
	return ServiceOfferingRecord{
		GUID:                 offering.Spec.GUID,
		Name:                 offering.Spec.GUID,
		Description:          offering.Spec.Description,
		Available:            offering.Spec.Available,
		Tags:                 offering.Spec.Tags,
		Requires:             offering.Spec.Requires,
		CreatedAt:            offering.Spec.CreationTimestamp.UTC().Format(TimestampFormat),
		UpdatedAt:            offering.Spec.UpdatedTimestamp.UTC().Format(TimestampFormat),
		Shareable:            offering.Spec.Shareable,
		DocumentationUrl:     offering.Spec.DocumentationUrl,
		BrokerId:             offering.OwnerReferences[0].Name,
		Bindable:             offering.Spec.Bindable,
		PlanUpdateable:       offering.Spec.PlanUpdateable,
		InstancesRetrievable: offering.Spec.InstancesRetrievable,
		BindingsRetrievable:  offering.Spec.BindingsRetrievable,
		AllowContextUpdates:  offering.Spec.AllowContextUpdates,
		CatalogId:            offering.Spec.CatalogId,
	}
}
