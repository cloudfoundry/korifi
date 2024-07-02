package payloads

import (
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
