package presenter

import (
	"net/url"

	"code.cloudfoundry.org/korifi/api/repositories"
)

const (
	serviceBrokersBase = "/v3/service_brokers"
)

type ServiceBrokerLinks struct {
	Self             Link `json:"self"`
	ServiceOfferings Link `json:"service_offerings"`
}

type ServiceBrokerResponse struct {
	repositories.ServiceBrokerResource
	Links ServiceBrokerLinks `json:"links"`
}

func ForServiceBroker(serviceBrokerResource repositories.ServiceBrokerResource, baseURL url.URL) ServiceBrokerResponse {
	return ServiceBrokerResponse{
		serviceBrokerResource,
		ServiceBrokerLinks{
			Self: Link{
				HRef: buildURL(baseURL).appendPath(serviceBrokersBase, serviceBrokerResource.GUID).build(),
			},
			ServiceOfferings: Link{
				HRef: buildURL(baseURL).appendPath(serviceOfferingsBase).setQuery("service_broker_guids=" + serviceBrokerResource.GUID).build(),
			},
		},
	}
}
