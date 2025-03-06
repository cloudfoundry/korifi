package payloads

import (
	"net/url"

	"code.cloudfoundry.org/korifi/api/payloads/parse"
	"code.cloudfoundry.org/korifi/api/payloads/validation"
	"code.cloudfoundry.org/korifi/api/repositories"
	jellidation "github.com/jellydator/validation"
)

type BrokerAuthentication struct {
	Type        string            `json:"type"`
	Credentials BrokerCredentials `json:"credentials"`
}

func (a BrokerAuthentication) Validate() error {
	return jellidation.ValidateStruct(&a,
		jellidation.Field(&a.Type, validation.OneOf("basic")),
		jellidation.Field(&a.Credentials, jellidation.Required),
	)
}

type BrokerCredentials struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (c BrokerCredentials) Validate() error {
	return jellidation.ValidateStruct(&c,
		jellidation.Field(&c.Username, jellidation.Required),
		jellidation.Field(&c.Password, jellidation.Required),
	)
}

type ServiceBrokerCreate struct {
	Name           string                `json:"name"`
	URL            string                `json:"url"`
	Labels         map[string]string     `json:"labels,omitempty"`
	Annotations    map[string]string     `json:"annotations,omitempty"`
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
		Name: c.Name,
		URL:  c.URL,
		Metadata: repositories.Metadata{
			Annotations: c.Annotations,
			Labels:      c.Labels,
		},
		Credentials: repositories.BrokerCredentials{
			Username: c.Authentication.Credentials.Username,
			Password: c.Authentication.Credentials.Password,
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
			Username: b.Authentication.Credentials.Username,
			Password: b.Authentication.Credentials.Password,
		}
	}

	return message
}
