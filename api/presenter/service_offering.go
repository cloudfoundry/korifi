package presenter

import (
	"net/url"

	"code.cloudfoundry.org/korifi/api/repositories"
	"code.cloudfoundry.org/korifi/model"
	"code.cloudfoundry.org/korifi/model/services"
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
	services.ServiceOffering
	model.CFResource
	Relationships ServiceOfferingRelationships `json:"relationships"`
	Links         ServiceOfferingLinks         `json:"links"`
}

type ServiceOfferingRelationships struct {
	ServiceBroker model.ToOneRelationship `json:"service_broker"`
}

func ForServiceOffering(serviceOffering repositories.ServiceOfferingRecord, baseURL url.URL) ServiceOfferingResponse {
	return ServiceOfferingResponse{
		ServiceOffering: serviceOffering.ServiceOffering,
		CFResource:      serviceOffering.CFResource,
		Relationships: ServiceOfferingRelationships{
			ServiceBroker: model.ToOneRelationship{
				Data: model.Relationship{
					GUID: serviceOffering.ServiceBrokerGUID,
				},
			},
		},
		Links: ServiceOfferingLinks{
			Self: Link{
				HRef: buildURL(baseURL).appendPath(serviceOfferingsBase, serviceOffering.GUID).build(),
			},
			ServicePlans: Link{
				HRef: buildURL(baseURL).appendPath(servicePlansBase).setQuery("service_offering_guids=" + serviceOffering.GUID).build(),
			},
			ServiceBroker: Link{
				HRef: buildURL(baseURL).appendPath(serviceBrokersBase, serviceOffering.ServiceBrokerGUID).build(),
			},
		},
	}
}
