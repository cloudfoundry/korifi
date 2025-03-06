package presenter

import (
	"net/url"
	"time"

	"code.cloudfoundry.org/korifi/api/repositories"
	"code.cloudfoundry.org/korifi/api/repositories/include"
)

const (
	serviceBrokersBase = "/v3/service_brokers"
)

type ServiceBrokerLinks struct {
	Self             Link `json:"self"`
	ServiceOfferings Link `json:"service_offerings"`
}

type ServiceBrokerResponse struct {
	GUID      string             `json:"guid"`
	Name      string             `json:"name"`
	URL       string             `json:"url"`
	CreatedAt time.Time          `json:"created_at"`
	UpdatedAt *time.Time         `json:"updated_at"`
	Metadata  Metadata           `json:"metadata"`
	Links     ServiceBrokerLinks `json:"links"`
}

func ForServiceBroker(serviceBrokerRecord repositories.ServiceBrokerRecord, baseURL url.URL, includes ...include.Resource) ServiceBrokerResponse {
	return ServiceBrokerResponse{
		GUID:      serviceBrokerRecord.GUID,
		Name:      serviceBrokerRecord.Name,
		URL:       serviceBrokerRecord.URL,
		CreatedAt: serviceBrokerRecord.CreatedAt,
		UpdatedAt: serviceBrokerRecord.UpdatedAt,
		Metadata: Metadata{
			Labels:      serviceBrokerRecord.Metadata.Labels,
			Annotations: serviceBrokerRecord.Metadata.Annotations,
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
