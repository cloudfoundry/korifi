package presenter

import (
	"net/url"

	"code.cloudfoundry.org/korifi/api/repositories"
)

const (
	serviceBrokersBase = "/v3/service_brokers"
)

type ServiceBrokerResponse struct {
	Name string `json:"name"`
	GUID string `json:"guid"`
	URL  string `json:"url"`

	CreatedAt     string             `json:"created_at"`
	UpdatedAt     string             `json:"updated_at"`
	Relationships Relationships      `json:"relationships"`
	Metadata      Metadata           `json:"metadata"`
	Links         ServiceBrokerLinks `json:"links"`
}

type ServiceBrokerLinks struct {
	Self             Link `json:"self"`
	ServiceOfferings Link `json:"service_offerings"`
}

func ForServiceBroker(serviceBrokerRecord repositories.ServiceBrokerRecord, baseURL url.URL) ServiceBrokerResponse {

	return ServiceBrokerResponse{
		Name:      serviceBrokerRecord.Name,
		GUID:      serviceBrokerRecord.GUID,
		URL:       serviceBrokerRecord.URL,
		CreatedAt: formatTimestamp(&serviceBrokerRecord.CreatedAt),
		UpdatedAt: formatTimestamp(serviceBrokerRecord.UpdatedAt),
		Metadata: Metadata{
			Labels:      emptyMapIfNil(serviceBrokerRecord.Labels),
			Annotations: emptyMapIfNil(serviceBrokerRecord.Annotations),
		},
		Links: ServiceBrokerLinks{
			Self: Link{
				HRef: buildURL(baseURL).appendPath(serviceBrokersBase, serviceBrokerRecord.GUID).build(),
			},
			ServiceOfferings: Link{
				HRef: buildURL(baseURL).appendPath(serviceOfferingsBase).setQuery("service_broker_guids=" + serviceBrokerRecord.GUID).build(),
			},
		},
	}
}

func ForServiceBrokerList(serviceBrokerRecordList []repositories.ServiceBrokerRecord, baseURL, requestURL url.URL) ListResponse[ServiceBrokerResponse] {
	return ForList(func(serviceBrokerRecord repositories.ServiceBrokerRecord, baseURL url.URL) ServiceBrokerResponse {
		return ForServiceBroker(serviceBrokerRecord, baseURL)
	}, serviceBrokerRecordList, baseURL, requestURL)
}
