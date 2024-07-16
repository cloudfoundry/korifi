package payloads

import (
	"net/url"

	"code.cloudfoundry.org/korifi/api/payloads/parse"
	"code.cloudfoundry.org/korifi/api/payloads/validation"
	"code.cloudfoundry.org/korifi/api/repositories"
	"code.cloudfoundry.org/korifi/model"
	"code.cloudfoundry.org/korifi/model/services"
	jellidation "github.com/jellydator/validation"
)

type BrokerAuthentication struct {
	Credentials services.BrokerCredentials `json:"credentials"`
	Type        string                     `json:"type"`
}

func (a BrokerAuthentication) Validate() error {
	return jellidation.ValidateStruct(&a,
		jellidation.Field(&a.Type, validation.OneOf("basic")),
	)
}

type ServiceBrokerCreate struct {
	services.ServiceBroker
	model.Metadata
	Authentication *BrokerAuthentication `json:"authentication"`
}

func (c ServiceBrokerCreate) Validate() error {
	return jellidation.ValidateStruct(&c,
		jellidation.Field(&c.Name, jellidation.Required),
		jellidation.Field(&c.URL, jellidation.Required),
		jellidation.Field(&c.Authentication, jellidation.Required),
	)
}

func (c ServiceBrokerCreate) ToCreateServiceBrokerMessage() repositories.CreateServiceBrokerMessage {
	return repositories.CreateServiceBrokerMessage{
		Broker:      c.ServiceBroker,
		Metadata:    c.Metadata,
		Credentials: c.Authentication.Credentials,
	}
}

type ServiceBrokerList struct {
	Names string
}

func (b *ServiceBrokerList) DecodeFromURLValues(values url.Values) error {
	b.Names = values.Get("names")
	return nil
}

func (b *ServiceBrokerList) SupportedKeys() []string {
	return []string{"names", "page", "per_page"}
}

func (b *ServiceBrokerList) ToMessage() repositories.ListServiceBrokerMessage {
	return repositories.ListServiceBrokerMessage{
		Names: parse.ArrayParam(b.Names),
	}
}
