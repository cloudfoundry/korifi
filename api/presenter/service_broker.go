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
	repositories.ServiceBrokerRecord
	Links ServiceBrokerLinks `json:"links"`
}

func ForServiceBroker(serviceBrokerRecord repositories.ServiceBrokerRecord, baseURL url.URL) ServiceBrokerResponse {
	return ServiceBrokerResponse{
		serviceBrokerRecord,
		ServiceBrokerLinks{
			Self: Link{
				HRef: buildURL(baseURL).appendPath(serviceBrokersBase, serviceBrokerRecord.GUID).build(),
			},
			ServiceOfferings: Link{
				HRef: buildURL(baseURL).appendPath(serviceOfferingsBase).setQuery("service_broker_guids=" + serviceBrokerRecord.GUID).build(),
			},
		},
	}
}
