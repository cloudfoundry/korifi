package payloads

import (
	"net/url"
	"regexp"

	"code.cloudfoundry.org/korifi/api/payloads/parse"
	"code.cloudfoundry.org/korifi/api/payloads/validation"
	"code.cloudfoundry.org/korifi/api/repositories"
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	jellidation "github.com/jellydator/validation"
)

type ServiceBrokerCreate struct {
	korifiv1alpha1.ServiceBroker
	Authentication korifiv1alpha1.BasicAuthentication `json:"authentication"`
}

func (c ServiceBrokerCreate) Validate() error {
	return jellidation.ValidateStruct(&c,
		jellidation.Field(&c.Name, jellidation.Required),
		jellidation.Field(&c.URL, jellidation.Required),
		jellidation.Field(&c.Authentication, jellidation.Required),
	)
}

type ServiceBrokerPatch struct {
	korifiv1alpha1.ServiceBrokerPatch
	Authentication *korifiv1alpha1.BasicAuthentication `json:"authentication"`
}

func (p ServiceBrokerPatch) Validate() error {
	return jellidation.ValidateStruct(&p,
		jellidation.Field(&p.Metadata),
	)
}

type ServiceBrokerList struct {
	Names         string
	GUIDs         string
	OrderBy       string
	LabelSelector string
}

func (l ServiceBrokerList) Validate() error {
	return jellidation.ValidateStruct(&l,
		jellidation.Field(&l.OrderBy, validation.OneOfOrderBy("created_at", "name", "updated_at")),
	)
}

func (l *ServiceBrokerList) ToMessage() repositories.ListServiceBrokerMessage {
	return repositories.ListServiceBrokerMessage{
		Names:         parse.ArrayParam(l.Names),
		GUIDs:         parse.ArrayParam(l.GUIDs),
		LabelSelector: l.LabelSelector,
	}
}

func (l *ServiceBrokerList) SupportedKeys() []string {
	return []string{"names", "guids", "order_by", "per_page", "page", "label_selector"}
}

func (l *ServiceBrokerList) IgnoredKeys() []*regexp.Regexp {
	return []*regexp.Regexp{regexp.MustCompile(`fields\[.+\]`)}
}

func (l *ServiceBrokerList) DecodeFromURLValues(values url.Values) error {
	l.Names = values.Get("names")
	l.GUIDs = values.Get("guids")
	l.OrderBy = values.Get("order_by")
	l.LabelSelector = values.Get("label_selector")
	return nil
}
