package presenter

import (
	"net/url"

	"code.cloudfoundry.org/korifi/api/repositories"
)

type ServicePlanLinks struct {
	Self            Link `json:"self"`
	ServiceOffering Link `json:"service_offering"`
}

type ServicePlanResponse struct {
	repositories.ServicePlanRecord
	Links ServicePlanLinks `json:"links"`
}

func ForServicePlan(servicePlan repositories.ServicePlanRecord, baseURL url.URL) ServicePlanResponse {
	return ServicePlanResponse{
		ServicePlanRecord: servicePlan,
		Links: ServicePlanLinks{
			Self: Link{
				HRef: buildURL(baseURL).appendPath(servicePlansBase, servicePlan.GUID).build(),
			},
			ServiceOffering: Link{
				HRef: buildURL(baseURL).appendPath(serviceOfferingsBase, servicePlan.Relationships.ServiceOffering.Data.GUID).build(),
			},
		},
	}
}
