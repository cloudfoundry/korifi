package payloads

import (
	"net/url"
	"regexp"

	"code.cloudfoundry.org/korifi/api/payloads/parse"
	"code.cloudfoundry.org/korifi/api/payloads/validation"
	"code.cloudfoundry.org/korifi/api/repositories"
	jellidation "github.com/jellydator/validation"
)

type ServiceOfferingList struct {
	Names               string
	BrokerNames         string
	IncludeBrokerFields []string
}

func (l ServiceOfferingList) Validate() error {
	return jellidation.ValidateStruct(&l,
		jellidation.Field(&l.IncludeBrokerFields, jellidation.Each(validation.OneOf("guid", "name"))),
	)
}

func (l *ServiceOfferingList) ToMessage() repositories.ListServiceOfferingMessage {
	return repositories.ListServiceOfferingMessage{
		Names:       parse.ArrayParam(l.Names),
		BrokerNames: parse.ArrayParam(l.BrokerNames),
	}
}

func (l *ServiceOfferingList) SupportedKeys() []string {
	return []string{"names", "service_broker_names", "fields[service_broker]", "page", "per_page"}
}

func (l *ServiceOfferingList) IgnoredKeys() []*regexp.Regexp {
	return nil
}

func (l *ServiceOfferingList) DecodeFromURLValues(values url.Values) error {
	l.Names = values.Get("names")
	l.BrokerNames = values.Get("service_broker_names")
	l.IncludeBrokerFields = parse.ArrayParam(values.Get("fields[service_broker]"))
	return nil
}
