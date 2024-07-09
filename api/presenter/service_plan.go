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
	repositories.ServicePlanResource
	Links ServicePlanLinks `json:"links"`
}

func ForServicePlan(servicePlanResource repositories.ServicePlanResource, baseURL url.URL) ServicePlanResponse {
	return ServicePlanResponse{
		ServicePlanResource: servicePlanResource,
		Links: ServicePlanLinks{
			Self: Link{
				HRef: buildURL(baseURL).appendPath(servicePlansBase, servicePlanResource.GUID).build(),
			},
			ServiceOffering: Link{
				HRef: buildURL(baseURL).appendPath(serviceOfferingsBase, servicePlanResource.Relationships.ServiceOffering.Data.GUID).build(),
			},
		},
	}
}

func ForServicePlanList(servicePlanResourceList []repositories.ServicePlanResource, baseURL, requestURL url.URL) ListResponse[ServicePlanResponse] {
	return ForList(func(servicePlanResource repositories.ServicePlanResource, baseURL url.URL) ServicePlanResponse {
		return ForServicePlan(servicePlanResource, baseURL)
	}, servicePlanResourceList, baseURL, requestURL)
}
