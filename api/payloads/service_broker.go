package payloads

import (
	"net/url"

	"code.cloudfoundry.org/korifi/api/payloads/parse"
	"code.cloudfoundry.org/korifi/api/payloads/validation"
	"code.cloudfoundry.org/korifi/api/repositories"
	"code.cloudfoundry.org/korifi/model"
	jellidation "github.com/jellydator/validation"
)

type BrokerAuthentication struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Type     string `json:"type"`
}

func (a BrokerAuthentication) Validate() error {
	return jellidation.ValidateStruct(&a,
		jellidation.Field(&a.Type, validation.OneOf("basic")),
		jellidation.Field(&a.Username, jellidation.Required),
		jellidation.Field(&a.Password, jellidation.Required),
	)
}

type ServiceBrokerCreate struct {
	Name string `json:"name"`
	URL  string `json:"url"`
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

func (c ServiceBrokerCreate) ToMessage() repositories.CreateServiceBrokerMessage {
	return repositories.CreateServiceBrokerMessage{
		Name:     c.Name,
		URL:      c.URL,
		Metadata: c.Metadata,
		Credentials: repositories.BrokerCredentials{
			Username: c.Authentication.Username,
			Password: c.Authentication.Password,
		},
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

type ServiceBrokerUpdate struct {
	Name           *string               `json:"name"`
	URL            *string               `json:"url"`
	Authentication *BrokerAuthentication `json:"authentication"`
	Metadata       MetadataPatch         `json:"metadata"`
}

func (c ServiceBrokerUpdate) Validate() error {
	return jellidation.ValidateStruct(&c,
		jellidation.Field(&c.Name),
		jellidation.Field(&c.URL),
		jellidation.Field(&c.Authentication),
	)
}

func (b *ServiceBrokerUpdate) IsAsyncRequest() bool {
	return b.Name != nil || b.URL != nil || b.Authentication != nil
}

func (b *ServiceBrokerUpdate) ToMessage(brokerGUID string) repositories.UpdateServiceBrokerMessage {
	message := repositories.UpdateServiceBrokerMessage{
		GUID:          brokerGUID,
		Name:          b.Name,
		URL:           b.URL,
		MetadataPatch: repositories.MetadataPatch(b.Metadata),
	}

	if b.Authentication != nil {
		message.Credentials = &repositories.BrokerCredentials{
			Username: b.Authentication.Username,
			Password: b.Authentication.Password,
		}
	}

	return message
}
