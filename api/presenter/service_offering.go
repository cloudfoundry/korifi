package presenter

import (
	"net/url"

	"code.cloudfoundry.org/korifi/api/repositories"
)

const (
	serviceOfferingsBase = "/v3/service_offerings"
	servicePlansBase     = "/v3/service_plans"
)

type ServiceOfferingLinks struct {
	Self          Link `json:"self"`
	ServicePlans  Link `json:"service_plans"`
	ServiceBroker Link `json:"service_broker"`
}

type ServiceOfferingResponse struct {
	repositories.ServiceOfferingRecord
	Links ServiceOfferingLinks `json:"links"`
}

func ForServiceOffering(serviceOffering repositories.ServiceOfferingRecord, baseURL url.URL) ServiceOfferingResponse {
	return ServiceOfferingResponse{
		ServiceOfferingRecord: serviceOffering,
		Links: ServiceOfferingLinks{
			Self: Link{
				HRef: buildURL(baseURL).appendPath(serviceOfferingsBase, serviceOffering.GUID).build(),
			},
			ServicePlans: Link{
				HRef: buildURL(baseURL).appendPath(servicePlansBase).setQuery("service_offering_guids=" + serviceOffering.GUID).build(),
			},
			ServiceBroker: Link{
				HRef: buildURL(baseURL).appendPath(serviceBrokersBase, serviceOffering.Relationships.ServiceBroker.Data.GUID).build(),
			},
		},
	}
}
