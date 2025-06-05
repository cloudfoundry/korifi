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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	ServicePlanResourceType           = "Service Plan"
	ServicePlanVisibilityResourceType = "Service Plan Visibility"
)

type ServicePlanRecord struct {
	GUID                string
	CreatedAt           time.Time
	UpdatedAt           *time.Time
	Metadata            Metadata
	Name                string
	Free                bool
	Description         string
	BrokerCatalog       ServicePlanBrokerCatalog
	Schemas             ServicePlanSchemas
	MaintenanceInfo     MaintenanceInfo
	Visibility          PlanVisibility
	ServiceOfferingGUID string
	Available           bool
}

type ServicePlanBrokerCatalog struct {
	ID       string
	Metadata map[string]any
	Features ServicePlanFeatures
}

type InputParameterSchema struct {
	Parameters map[string]any
}

type ServiceInstanceSchema struct {
	Create InputParameterSchema
	Update InputParameterSchema
}

type ServiceBindingSchema struct {
	Create InputParameterSchema
}

type ServicePlanSchemas struct {
	ServiceInstance ServiceInstanceSchema
	ServiceBinding  ServiceBindingSchema
}

type ServicePlanFeatures struct {
	PlanUpdateable bool
	Bindable       bool
}

type MaintenanceInfo struct {
	Version string
}

func (r ServicePlanRecord) Relationships() map[string]string {
	return map[string]string{
		"service_offering": r.ServiceOfferingGUID,
	}
}

type PlanVisibility struct {
	Type          string
	Organizations []VisibilityOrganization
}

type VisibilityOrganization struct {
	GUID string
	Name string
}

type ServicePlanRepo struct {
	klient        Klient
	rootNamespace string
	orgRepo       *OrgRepo
}

type ListServicePlanMessage struct {
	GUIDs                []string
	Names                []string
	ServiceOfferingGUIDs []string
	ServiceOfferingNames []string
	BrokerNames          []string
	BrokerGUIDs          []string
	Available            *bool
}

func (m *ListServicePlanMessage) toListOptions(rootNamespace string) []ListOption {
	return []ListOption{
		InNamespace(rootNamespace),
		WithLabelIn(korifiv1alpha1.GUIDLabelKey, m.GUIDs),
		WithLabelIn(korifiv1alpha1.CFServicePlanNameKey, tools.EncodeValuesToSha224(m.Names...)),
		WithLabelIn(korifiv1alpha1.RelServiceOfferingGUIDLabel, m.ServiceOfferingGUIDs),
		WithLabelIn(korifiv1alpha1.RelServiceBrokerNameLabel, tools.EncodeValuesToSha224(m.BrokerNames...)),
		WithLabelIn(korifiv1alpha1.RelServiceBrokerGUIDLabel, m.BrokerGUIDs),
		WithLabelIn(korifiv1alpha1.RelServiceOfferingNameLabel, tools.EncodeValuesToSha224(m.ServiceOfferingNames...)),
		m.toAvailableListOption(),
	}
}

func (m *ListServicePlanMessage) toAvailableListOption() ListOption {
	if m.Available == nil {
		return NoopListOption{}
	}

	if *m.Available {
		return WithLabel(korifiv1alpha1.CFServicePlanAvailableKey, "true")
	}

	return WithLabel(korifiv1alpha1.CFServicePlanAvailableKey, "false")
}

type ApplyServicePlanVisibilityMessage struct {
	PlanGUID      string
	Type          string
	Organizations []string
}

func (m *ApplyServicePlanVisibilityMessage) apply(cfServicePlan *korifiv1alpha1.CFServicePlan) {
	cfServicePlan.Spec.Visibility.Type = m.Type
	cfServicePlan.Spec.Visibility.Organizations = tools.Uniq(append(
		cfServicePlan.Spec.Visibility.Organizations,
		m.Organizations...,
	))
	if m.Type != korifiv1alpha1.OrganizationServicePlanVisibilityType {
		cfServicePlan.Spec.Visibility.Organizations = []string{}
	}
}

type UpdateServicePlanVisibilityMessage struct {
	PlanGUID      string
	Type          string
	Organizations []string
}

func (m *UpdateServicePlanVisibilityMessage) apply(cfServicePlan *korifiv1alpha1.CFServicePlan) {
	cfServicePlan.Spec.Visibility.Type = m.Type
	cfServicePlan.Spec.Visibility.Organizations = tools.Uniq(m.Organizations)
}

type DeleteServicePlanVisibilityMessage struct {
	PlanGUID string
	OrgGUID  string
}

func (m *DeleteServicePlanVisibilityMessage) apply(cfServicePlan *korifiv1alpha1.CFServicePlan) {
	for i, org := range cfServicePlan.Spec.Visibility.Organizations {
		if org == m.OrgGUID {
			cfServicePlan.Spec.Visibility.Organizations = append(cfServicePlan.Spec.Visibility.Organizations[:i], cfServicePlan.Spec.Visibility.Organizations[i+1:]...)
		}
	}
}

func NewServicePlanRepo(
	klient Klient,
	rootNamespace string,
	orgRepo *OrgRepo,
) *ServicePlanRepo {
	return &ServicePlanRepo{
		klient:        klient,
		rootNamespace: rootNamespace,
		orgRepo:       orgRepo,
	}
}

func (r *ServicePlanRepo) ListPlans(ctx context.Context, authInfo authorization.Info, message ListServicePlanMessage) ([]ServicePlanRecord, error) {
	cfServicePlans := &korifiv1alpha1.CFServicePlanList{}
	if err := r.klient.List(ctx, cfServicePlans, message.toListOptions(r.rootNamespace)...); err != nil {
		return nil, apierrors.FromK8sError(err, ServicePlanResourceType)
	}

	return it.TryCollect(it.MapError(slices.Values(cfServicePlans.Items), func(plan korifiv1alpha1.CFServicePlan) (ServicePlanRecord, error) {
		return r.planToRecord(ctx, authInfo, plan)
	}))
}

func (r *ServicePlanRepo) GetPlan(ctx context.Context, authInfo authorization.Info, planGUID string) (ServicePlanRecord, error) {
	cfServicePlan := &korifiv1alpha1.CFServicePlan{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: r.rootNamespace,
			Name:      planGUID,
		},
	}

	err := r.klient.Get(ctx, cfServicePlan)
	if err != nil {
		return ServicePlanRecord{}, apierrors.FromK8sError(err, ServicePlanVisibilityResourceType)
	}
	return r.planToRecord(ctx, authInfo, *cfServicePlan)
}

func (r *ServicePlanRepo) ApplyPlanVisibility(ctx context.Context, authInfo authorization.Info, message ApplyServicePlanVisibilityMessage) (ServicePlanRecord, error) {
	return r.patchServicePlan(ctx, authInfo, message.PlanGUID, message.apply)
}

func (r *ServicePlanRepo) UpdatePlanVisibility(ctx context.Context, authInfo authorization.Info, message UpdateServicePlanVisibilityMessage) (ServicePlanRecord, error) {
	return r.patchServicePlan(ctx, authInfo, message.PlanGUID, message.apply)
}

func (r *ServicePlanRepo) DeletePlanVisibility(ctx context.Context, authInfo authorization.Info, message DeleteServicePlanVisibilityMessage) error {
	if _, err := r.patchServicePlan(ctx, authInfo, message.PlanGUID, message.apply); err != nil {
		return err
	}

	return nil
}

func (r *ServicePlanRepo) DeletePlan(ctx context.Context, authInfo authorization.Info, planGUID string) error {
	cfServicePlan := &korifiv1alpha1.CFServicePlan{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: r.rootNamespace,
			Name:      planGUID,
		},
	}

	if err := r.klient.Delete(ctx, cfServicePlan); err != nil {
		return apierrors.FromK8sError(err, ServicePlanResourceType)
	}

	return nil
}

func (r *ServicePlanRepo) patchServicePlan(
	ctx context.Context,
	authInfo authorization.Info,
	planGUID string,
	patchFunc func(*korifiv1alpha1.CFServicePlan),
) (ServicePlanRecord, error) {
	cfServicePlan := &korifiv1alpha1.CFServicePlan{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: r.rootNamespace,
			Name:      planGUID,
		},
	}

	if err := GetAndPatch(ctx, r.klient, cfServicePlan, func() error {
		patchFunc(cfServicePlan)
		return nil
	}); err != nil {
		return ServicePlanRecord{}, apierrors.FromK8sError(err, ServicePlanVisibilityResourceType)
	}

	return r.planToRecord(ctx, authInfo, *cfServicePlan)
}

func (r *ServicePlanRepo) planToRecord(ctx context.Context, authInfo authorization.Info, plan korifiv1alpha1.CFServicePlan) (ServicePlanRecord, error) {
	organizations := []VisibilityOrganization{}
	if plan.Spec.Visibility.Type == korifiv1alpha1.OrganizationServicePlanVisibilityType {
		var err error
		organizations, err = r.toVisibilityOrganizations(ctx, authInfo, plan.Spec.Visibility.Organizations)
		if err != nil {
			return ServicePlanRecord{}, err
		}
	}

	metadata, err := korifiv1alpha1.AsMap(plan.Spec.BrokerCatalog.Metadata)
	if err != nil {
		return ServicePlanRecord{}, err
	}

	instanceCreateParameters, err := korifiv1alpha1.AsMap(plan.Spec.Schemas.ServiceInstance.Create.Parameters)
	if err != nil {
		return ServicePlanRecord{}, err
	}

	instanceUpdateParameters, err := korifiv1alpha1.AsMap(plan.Spec.Schemas.ServiceInstance.Update.Parameters)
	if err != nil {
		return ServicePlanRecord{}, err
	}

	bindingCreateParameters, err := korifiv1alpha1.AsMap(plan.Spec.Schemas.ServiceBinding.Create.Parameters)
	if err != nil {
		return ServicePlanRecord{}, err
	}

	return ServicePlanRecord{
		Name:        plan.Spec.Name,
		Free:        plan.Spec.Free,
		Description: plan.Spec.Description,
		BrokerCatalog: ServicePlanBrokerCatalog{
			ID:       plan.Spec.BrokerCatalog.ID,
			Metadata: metadata,
			Features: ServicePlanFeatures(plan.Spec.BrokerCatalog.Features),
		},
		Schemas: ServicePlanSchemas{
			ServiceInstance: ServiceInstanceSchema{
				Create: InputParameterSchema{
					Parameters: instanceCreateParameters,
				},
				Update: InputParameterSchema{
					Parameters: instanceUpdateParameters,
				},
			},
			ServiceBinding: ServiceBindingSchema{
				Create: InputParameterSchema{
					Parameters: bindingCreateParameters,
				},
			},
		},
		MaintenanceInfo: MaintenanceInfo(plan.Spec.MaintenanceInfo),
		GUID:            plan.Name,
		CreatedAt:       plan.CreationTimestamp.Time,
		Metadata: Metadata{
			Labels:      plan.Labels,
			Annotations: plan.Annotations,
		},
		Visibility: PlanVisibility{
			Type:          plan.Spec.Visibility.Type,
			Organizations: organizations,
		},
		ServiceOfferingGUID: plan.Labels[korifiv1alpha1.RelServiceOfferingGUIDLabel],
		Available:           plan.Labels[korifiv1alpha1.CFServicePlanAvailableKey] == "true",
	}, nil
}

func (r *ServicePlanRepo) toVisibilityOrganizations(ctx context.Context, authInfo authorization.Info, orgGUIDs []string) ([]VisibilityOrganization, error) {
	orgs, err := r.orgRepo.ListOrgs(ctx, authInfo, ListOrgsMessage{
		GUIDs: orgGUIDs,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list orgs for plan visibility: %w", err)
	}

	return slices.Collect(it.Map(slices.Values(orgs), func(o OrgRecord) VisibilityOrganization {
		return VisibilityOrganization{
			GUID: o.GUID,
			Name: o.Name,
		}
	})), nil
}
