package repositories

import (
	"context"
	"fmt"

	"code.cloudfoundry.org/korifi/api/authorization"
	apierrors "code.cloudfoundry.org/korifi/api/errors"
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/model"
	"code.cloudfoundry.org/korifi/model/services"
	"github.com/BooleanCat/go-functional/iter"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	ServicePlanResourceType           = "Service Plan"
	ServicePlanVisibilityResourceType = "Service Plan Visibility"
)

type ServicePlanRecord struct {
	services.ServicePlan
	model.CFResource
	VisibilityType string                   `json:"visibility_type"`
	Relationships  ServicePlanRelationships `json:"relationships"`
}

type ServicePlanRelationships struct {
	ServiceOffering model.ToOneRelationship `json:"service_offering"`
}

type ServicePlanRepo struct {
	userClientFactory authorization.UserK8sClientFactory
	rootNamespace     string
}

type ListServicePlanMessage struct {
	ServiceOfferingGUIDs []string
}

func (m *ListServicePlanMessage) matches(cfServicePlan korifiv1alpha1.CFServicePlan) bool {
	return emptyOrContains(m.ServiceOfferingGUIDs, cfServicePlan.Labels[korifiv1alpha1.RelServiceOfferingLabel])
}

type ApplyServicePlanVisibilityMessage struct {
	PlanGUID string
	Type     string
}

func NewServicePlanRepo(
	userClientFactory authorization.UserK8sClientFactory,
	rootNamespace string,
) *ServicePlanRepo {
	return &ServicePlanRepo{
		userClientFactory: userClientFactory,
		rootNamespace:     rootNamespace,
	}
}

func (r *ServicePlanRepo) ListPlans(ctx context.Context, authInfo authorization.Info, message ListServicePlanMessage) ([]ServicePlanRecord, error) {
	userClient, err := r.userClientFactory.BuildClient(authInfo)
	if err != nil {
		return nil, fmt.Errorf("failed to build user client: %w", err)
	}

	cfServicePlans := &korifiv1alpha1.CFServicePlanList{}
	if err := userClient.List(ctx, cfServicePlans, client.InNamespace(r.rootNamespace)); err != nil {
		return nil, apierrors.FromK8sError(err, ServicePlanResourceType)
	}

	return iter.Map(iter.Lift(cfServicePlans.Items).Filter(message.matches), planToRecord).Collect(), nil
}

func (r *ServicePlanRepo) GetPlan(ctx context.Context, authInfo authorization.Info, planGUID string) (ServicePlanRecord, error) {
	userClient, err := r.userClientFactory.BuildClient(authInfo)
	if err != nil {
		return ServicePlanRecord{}, fmt.Errorf("failed to build user client: %w", err)
	}

	cfServicePlan := &korifiv1alpha1.CFServicePlan{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: r.rootNamespace,
			Name:      planGUID,
		},
	}

	err = userClient.Get(ctx, client.ObjectKeyFromObject(cfServicePlan), cfServicePlan)
	if err != nil {
		return ServicePlanRecord{}, apierrors.FromK8sError(err, ServicePlanVisibilityResourceType)
	}
	return planToRecord(*cfServicePlan), nil
}

func (r *ServicePlanRepo) ApplyPlanVisibility(ctx context.Context, authInfo authorization.Info, message ApplyServicePlanVisibilityMessage) (ServicePlanRecord, error) {
	userClient, err := r.userClientFactory.BuildClient(authInfo)
	if err != nil {
		return ServicePlanRecord{}, fmt.Errorf("failed to build user client: %w", err)
	}

	cfServicePlan := &korifiv1alpha1.CFServicePlan{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: r.rootNamespace,
			Name:      message.PlanGUID,
		},
	}

	if err := PatchResource(ctx, userClient, cfServicePlan, func() {
		cfServicePlan.Spec.Visibility.Type = message.Type
	}); err != nil {
		return ServicePlanRecord{}, apierrors.FromK8sError(err, ServicePlanVisibilityResourceType)
	}

	return planToRecord(*cfServicePlan), nil
}

func planToRecord(plan korifiv1alpha1.CFServicePlan) ServicePlanRecord {
	return ServicePlanRecord{
		ServicePlan: plan.Spec.ServicePlan,
		CFResource: model.CFResource{
			GUID:      plan.Name,
			CreatedAt: plan.CreationTimestamp.Time,
			Metadata: model.Metadata{
				Labels:      plan.Labels,
				Annotations: plan.Annotations,
			},
		},
		VisibilityType: plan.Spec.Visibility.Type,
		Relationships: ServicePlanRelationships{
			ServiceOffering: model.ToOneRelationship{
				Data: model.Relationship{
					GUID: plan.Labels[korifiv1alpha1.RelServiceOfferingLabel],
				},
			},
		},
	}
}
