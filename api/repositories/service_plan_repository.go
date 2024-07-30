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
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const ServicePlanResourceType = "Service Plan"

type ServicePlanRecord struct {
	services.ServicePlan
	model.CFResource
	Relationships ServicePlanRelationships `json:"relationships"`
}

type ServicePlanRelationships struct {
	ServiceOffering model.ToOneRelationship `json:"service_offering"`
}

type ServicePlanRepo struct {
	userClientFactory authorization.UserK8sClientFactory
	rootNamespace     string
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

type ListServicePlanMessage struct {
	ServiceOfferingGUIDs []string
}

func (m *ListServicePlanMessage) matches(cfServicePlan korifiv1alpha1.CFServicePlan) bool {
	return emptyOrContains(m.ServiceOfferingGUIDs, cfServicePlan.Labels[korifiv1alpha1.RelServiceOfferingLabel])
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
		Relationships: ServicePlanRelationships{
			ServiceOffering: model.ToOneRelationship{
				Data: model.Relationship{
					GUID: plan.Labels[korifiv1alpha1.RelServiceOfferingLabel],
				},
			},
		},
	}
}
