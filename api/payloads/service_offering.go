package payloads

import (
	"net/url"
	"regexp"

	"code.cloudfoundry.org/korifi/api/payloads/parse"
	"code.cloudfoundry.org/korifi/api/repositories"
)

type ServiceOfferingList struct {
	Names string
}

func (l *ServiceOfferingList) ToMessage() repositories.ListServiceOfferingMessage {
	return repositories.ListServiceOfferingMessage{
		Names: parse.ArrayParam(l.Names),
	}
}

func (l *ServiceOfferingList) SupportedKeys() []string {
	return []string{"names", "page", "per_page"}
}

func (l *ServiceOfferingList) IgnoredKeys() []*regexp.Regexp {
	return []*regexp.Regexp{regexp.MustCompile(`fields\[.+\]`)}
}

func (l *ServiceOfferingList) DecodeFromURLValues(values url.Values) error {
	l.Names = values.Get("names")
	return nil
}
