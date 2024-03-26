package repositories

import (
	"context"
	"fmt"

	"code.cloudfoundry.org/korifi/api/authorization"
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/tools/k8s"

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
	Available            *bool
	SpaceGuids           []string
	ServiceOfferingNames []string
	ServiceOfferingGUIDs []string
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

func toServiceOfferingResource(cfServiceOffering *korifiv1alpha1.CFServiceOffering) korifiv1alpha1.ServiceOfferingResource {
	rels := korifiv1alpha1.ServiceOfferingRelationships{}
	rels.Create(cfServiceOffering)

	return korifiv1alpha1.ServiceOfferingResource{
		ServiceOffering: cfServiceOffering.Spec.ServiceOffering,
		CFResource: korifiv1alpha1.CFResource{
			GUID: cfServiceOffering.Name,
		},
		Relationships: rels,
	}
}

func toServicePlanResource(cfServicePlan *korifiv1alpha1.CFServicePlan) korifiv1alpha1.ServicePlanResource {
	rels := korifiv1alpha1.ServicePlanRelationships{}
	rels.Create(cfServicePlan)

	return korifiv1alpha1.ServicePlanResource{
		ServicePlan:    cfServicePlan.Spec.ServicePlan,
		CFResource:     korifiv1alpha1.CFResource{GUID: cfServicePlan.Name},
		Available:      cfServicePlan.IsAvailable(),
		VisibilityType: cfServicePlan.Spec.Visibility.Type,
		Relationships:  rels,
	}
}

type PlanVisibilityApplyMessage struct {
	PlanGUID      string
	Type          string
	Organizations []string
}

func (r *ServiceCatalogRepo) ListServiceOfferings(ctx context.Context, authInfo authorization.Info, message ListServiceOfferingMessage) ([]korifiv1alpha1.ServiceOfferingResource, error) {
	userClient, err := r.userClientFactory.BuildClient(authInfo)
	if err != nil {
		return []korifiv1alpha1.ServiceOfferingResource{}, fmt.Errorf("failed to build user client: %w", err)
	}

	var result []korifiv1alpha1.ServiceOfferingResource

	allServiceOferings := &korifiv1alpha1.CFServiceOfferingList{}
	err = userClient.List(ctx, allServiceOferings, client.InNamespace(r.rootNamespace))
	if err != nil {
		return []korifiv1alpha1.ServiceOfferingResource{}, fmt.Errorf("failed to list service offerings: %w", err)
	}

	for _, o := range allServiceOferings.Items {
		if !filterAppliesTo(o.Spec.Name, message.Names) {
			continue
		}

		result = append(result, toServiceOfferingResource(&o))

	}

	return result, nil
}

func (r *ServiceCatalogRepo) GetServiceOffering(ctx context.Context, authInfo authorization.Info, guid string) (korifiv1alpha1.ServiceOfferingResource, error) {
	userClient, err := r.userClientFactory.BuildClient(authInfo)
	if err != nil {
		return korifiv1alpha1.ServiceOfferingResource{}, fmt.Errorf("failed to build user client: %w", err)
	}

	serviceOffering := &korifiv1alpha1.CFServiceOffering{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: r.rootNamespace,
			Name:      guid,
		},
	}
	err = userClient.Get(ctx, client.ObjectKeyFromObject(serviceOffering), serviceOffering)
	if err != nil {
		return korifiv1alpha1.ServiceOfferingResource{}, fmt.Errorf("failed to get service offering: %w", err)
	}

	return toServiceOfferingResource(serviceOffering), nil
}

func (r *ServiceCatalogRepo) ListServicePlans(ctx context.Context, authInfo authorization.Info, message ListServicePlanMessage) ([]korifiv1alpha1.ServicePlanResource, error) {
	userClient, err := r.userClientFactory.BuildClient(authInfo)
	if err != nil {
		return nil, fmt.Errorf("failed to build user client: %w", err)
	}

	var result []korifiv1alpha1.ServicePlanResource

	allServicePlans := &korifiv1alpha1.CFServicePlanList{}
	err = userClient.List(ctx, allServicePlans, client.InNamespace(r.rootNamespace))
	if err != nil {
		return nil, fmt.Errorf("failed to list service plans: %w", err)
	}

	offeringGuids, err := r.getOfferingGuids(ctx, userClient, message.ServiceOfferingNames)
	if err != nil {
		return nil, fmt.Errorf("failed to list service offerings: %w", err)
	}

	offeringGuids = append(offeringGuids, message.ServiceOfferingGUIDs...)

	for _, p := range allServicePlans.Items {
		if !filterAppliesTo(p.Spec.Name, message.Names) {
			continue
		}

		if message.Available != nil && *message.Available != p.IsAvailable() {
			continue
		}

		if !filterAppliesTo(p.Labels[korifiv1alpha1.RelServiceOfferingLabel], offeringGuids) {
			continue
		}

		result = append(result, toServicePlanResource(&p))

	}

	return result, nil
}

func (r *ServiceCatalogRepo) GetServicePlan(ctx context.Context, authInfo authorization.Info, guid string) (korifiv1alpha1.ServicePlanResource, error) {
	userClient, err := r.userClientFactory.BuildClient(authInfo)
	if err != nil {
		return korifiv1alpha1.ServicePlanResource{}, fmt.Errorf("failed to build user client: %w", err)
	}

	servicePlan, err := r.getServicePlan(ctx, userClient, guid)
	if err != nil {
		return korifiv1alpha1.ServicePlanResource{}, fmt.Errorf("failed to get service plan: %w", err)
	}

	return toServicePlanResource(servicePlan), nil
}

func (r *ServiceCatalogRepo) getServicePlan(ctx context.Context, userClient client.Client, guid string) (*korifiv1alpha1.CFServicePlan, error) {
	servicePlan := &korifiv1alpha1.CFServicePlan{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: r.rootNamespace,
			Name:      guid,
		},
	}
	err := userClient.Get(ctx, client.ObjectKeyFromObject(servicePlan), servicePlan)
	if err != nil {
		return nil, fmt.Errorf("failed to get service plan: %w", err)
	}

	return servicePlan, nil
}

func (r *ServiceCatalogRepo) ApplyPlanVisibility(
	ctx context.Context,
	authInfo authorization.Info,
	visibilityMessage PlanVisibilityApplyMessage,
) (korifiv1alpha1.ServicePlanVisibilityResource, error) {
	userClient, err := r.userClientFactory.BuildClient(authInfo)
	if err != nil {
		return korifiv1alpha1.ServicePlanVisibilityResource{}, fmt.Errorf("failed to build user client: %w", err)
	}

	servicePlan, err := r.getServicePlan(ctx, userClient, visibilityMessage.PlanGUID)
	if err != nil {
		return korifiv1alpha1.ServicePlanVisibilityResource{}, fmt.Errorf("failed to get service plan: %w", err)
	}

	vilibilityOrgs := []korifiv1alpha1.VisibilityOrganization{}
	for _, orgGUID := range visibilityMessage.Organizations {
		org := &korifiv1alpha1.CFOrg{
			ObjectMeta: metav1.ObjectMeta{
				Name:      orgGUID,
				Namespace: r.rootNamespace,
			},
		}

		err := userClient.Get(ctx, client.ObjectKeyFromObject(org), org)
		if err != nil {
			return korifiv1alpha1.ServicePlanVisibilityResource{}, fmt.Errorf("failed to get org %q: %w", orgGUID, err)
		}

		vilibilityOrgs = append(vilibilityOrgs, korifiv1alpha1.VisibilityOrganization{
			GUID: orgGUID,
			Name: org.Spec.DisplayName,
		})
	}

	err = k8s.PatchResource(ctx, userClient, servicePlan, func() {
		servicePlan.Spec.Visibility.Type = visibilityMessage.Type
		servicePlan.Spec.Visibility.Organizations = vilibilityOrgs
	})
	if err != nil {
		return korifiv1alpha1.ServicePlanVisibilityResource{}, fmt.Errorf("failed to patch service plan: %w", err)
	}
	return korifiv1alpha1.ServicePlanVisibilityResource{
		Type: visibilityMessage.Type,
	}, nil
}

func (r *ServiceCatalogRepo) GetPlanVisibility(
	ctx context.Context,
	authInfo authorization.Info,
	planGUID string,
) (korifiv1alpha1.ServicePlanVisibilityResource, error) {
	userClient, err := r.userClientFactory.BuildClient(authInfo)
	if err != nil {
		return korifiv1alpha1.ServicePlanVisibilityResource{}, fmt.Errorf("failed to build user client: %w", err)
	}

	servicePlan, err := r.getServicePlan(ctx, userClient, planGUID)
	if err != nil {
		return korifiv1alpha1.ServicePlanVisibilityResource{}, fmt.Errorf("failed to get service plan: %w", err)
	}

	return korifiv1alpha1.ServicePlanVisibilityResource{
		Type:          servicePlan.Spec.Visibility.Type,
		Organizations: servicePlan.Spec.Visibility.Organizations,
	}, nil
}

func (r *ServiceCatalogRepo) getOfferingGuids(ctx context.Context, userClient client.Client, names []string) ([]string, error) {
	if len(names) == 0 {
		return []string{}, nil
	}

	offerings := &korifiv1alpha1.CFServiceOfferingList{}
	err := userClient.List(ctx, offerings, client.InNamespace(r.rootNamespace))
	if err != nil {
		return nil, err
	}

	guids := []string{}
	for _, o := range offerings.Items {
		if !filterAppliesTo(o.Spec.Name, names) {
			continue
		}
		guids = append(guids, o.Name)

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
