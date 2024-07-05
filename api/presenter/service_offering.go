package presenter

import (
	"net/url"

	"code.cloudfoundry.org/korifi/api/repositories"
)

const (
	serviceOfferingsBase = "/v3/service_offerings"
	servicePlansBase     = "/v3/service_plans"
	serviceBrokersBase   = "/v3/service_brokers"
)

type ServiceOfferingLinks struct {
	Self          Link `json:"self"`
	ServicePlans  Link `json:"service_plans"`
	ServiceBroker Link `json:"service_broker"`
}

type ServiceOfferingResponse struct {
	repositories.ServiceOfferingResource
	Links ServiceOfferingLinks `json:"links"`
}

func ForServiceOffering(serviceOfferingResource repositories.ServiceOfferingResource, baseURL url.URL) ServiceOfferingResponse {
	return ServiceOfferingResponse{
		ServiceOfferingResource: serviceOfferingResource,
		Links: ServiceOfferingLinks{
			Self: Link{
				HRef: buildURL(baseURL).appendPath(serviceOfferingsBase, serviceOfferingResource.GUID).build(),
			},
			ServicePlans: Link{
				HRef: buildURL(baseURL).appendPath(servicePlansBase).setQuery("service_offering_guids=" + serviceOfferingResource.GUID).build(),
			},
			ServiceBroker: Link{
				HRef: buildURL(baseURL).appendPath(serviceBrokersBase, serviceOfferingResource.Relationships.ServiceBroker.Data.GUID).build(),
			},
		},
	}
}

func ForServiceOfferingList(serviceOfferingResourceList []repositories.ServiceOfferingResource, baseURL, requestURL url.URL) ListResponse[ServiceOfferingResponse] {
	return ForList(func(serviceOfferingResource repositories.ServiceOfferingResource, baseURL url.URL) ServiceOfferingResponse {
		return ForServiceOffering(serviceOfferingResource, baseURL)
	}, serviceOfferingResourceList, baseURL, requestURL)
}
