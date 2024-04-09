package presenter

import (
	"net/url"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
)

const (
	serviceBrokersBase = "/v3/service_brokers"
)

type ServiceBrokerLinks struct {
	Self             Link `json:"self"`
	ServiceOfferings Link `json:"service_offerings"`
}

type ServiceBrokerResponse struct {
	korifiv1alpha1.ServiceBrokerResource
	Links ServiceBrokerLinks `json:"links"`
}

func ForServiceBroker(serviceBrokerResource korifiv1alpha1.ServiceBrokerResource, baseURL url.URL) ServiceBrokerResponse {

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

func ForServiceBrokerList(serviceBrokerResourceList []korifiv1alpha1.ServiceBrokerResource, baseURL, requestURL url.URL) ListResponse[ServiceBrokerResponse] {
	return ForList(func(serviceBrokerResource korifiv1alpha1.ServiceBrokerResource, baseURL url.URL) ServiceBrokerResponse {
		return ForServiceBroker(serviceBrokerResource, baseURL)
	}, serviceBrokerResourceList, baseURL, requestURL)
}
