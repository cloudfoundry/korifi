package relationships

import (
	"context"
	"fmt"
	"slices"

	"code.cloudfoundry.org/korifi/api/authorization"
	"code.cloudfoundry.org/korifi/api/repositories"
	"github.com/BooleanCat/go-functional/v2/it"
	"github.com/BooleanCat/go-functional/v2/it/itx"
)

//counterfeiter:generate -o fake -fake-name ServiceOfferingRepository . ServiceOfferingRepository
type ServiceOfferingRepository interface {
	ListOfferings(context.Context, authorization.Info, repositories.ListServiceOfferingMessage) ([]repositories.ServiceOfferingRecord, error)
}

//counterfeiter:generate -o fake -fake-name ServiceBrokerRepository . ServiceBrokerRepository
type ServiceBrokerRepository interface {
	ListServiceBrokers(context.Context, authorization.Info, repositories.ListServiceBrokerMessage) ([]repositories.ServiceBrokerRecord, error)
}

//counterfeiter:generate -o fake -fake-name ServicePlanRepository . ServicePlanRepository
type ServicePlanRepository interface {
	ListPlans(context.Context, authorization.Info, repositories.ListServicePlanMessage) ([]repositories.ServicePlanRecord, error)
}

//counterfeiter:generate -o fake -fake-name Resource . Resource
type Resource interface {
	Relationships() map[string]string
}

type ResourceRelationshipsRepo struct {
	serviceOfferingRepo ServiceOfferingRepository
	serviceBrokerRepo   ServiceBrokerRepository
	servicePlanRepo     ServicePlanRepository
}

func NewResourseRelationshipsRepo(
	serviceOfferingRepo ServiceOfferingRepository,
	serviceBrokerRepo ServiceBrokerRepository,
	servicePlanRepo ServicePlanRepository,
) *ResourceRelationshipsRepo {
	return &ResourceRelationshipsRepo{
		serviceOfferingRepo: serviceOfferingRepo,
		serviceBrokerRepo:   serviceBrokerRepo,
		servicePlanRepo:     servicePlanRepo,
	}
}

func (r *ResourceRelationshipsRepo) ListRelatedResources(ctx context.Context, authInfo authorization.Info, relatedResourceType string, resources []Resource) ([]Resource, error) {
	relatedResourceGUIDs := slices.Collect(it.Exclude(it.Map(itx.FromSlice(resources), func(r Resource) string {
		return r.Relationships()[relatedResourceType]
	}), func(guid string) bool {
		return guid == ""
	}))

	if len(relatedResourceGUIDs) == 0 {
		return nil, nil
	}

	switch relatedResourceType {
	case "service_offering":
		return asResources(r.serviceOfferingRepo.ListOfferings(
			ctx,
			authInfo,
			repositories.ListServiceOfferingMessage{GUIDs: relatedResourceGUIDs},
		))

	case "service_broker":
		return asResources(r.serviceBrokerRepo.ListServiceBrokers(
			ctx,
			authInfo,
			repositories.ListServiceBrokerMessage{GUIDs: relatedResourceGUIDs},
		))
	case "service_plan":
		return asResources(r.servicePlanRepo.ListPlans(
			ctx,
			authInfo,
			repositories.ListServicePlanMessage{GUIDs: relatedResourceGUIDs},
		))
	case "space", "organization":
		return []Resource{}, nil
	}

	return nil, fmt.Errorf("no repository for type %q", relatedResourceType)
}

func asResources[S ~[]E, E Resource](resources S, err error) ([]Resource, error) {
	return slices.Collect(it.Map(itx.FromSlice(resources), func(o E) Resource {
		return o
	})), err
}
