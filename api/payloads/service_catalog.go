package payloads

import (
	"net/url"

	"code.cloudfoundry.org/korifi/api/repositories"
)

type ServiceOfferingList struct {
	Names string
}

func (l *ServiceOfferingList) ToMessage() repositories.ListServiceOfferingMessage {
	return repositories.ListServiceOfferingMessage{
		Names: ParseArrayParam(l.Names),
	}
}

func (l *ServiceOfferingList) SupportedKeys() []string {
	return []string{"names", "page", "space_guids"}
}

func (l *ServiceOfferingList) DecodeFromURLValues(values url.Values) error {
	l.Names = values.Get("names")
	return nil
}

type ServicePlanList struct {
	Names                string
	ServiceOfferingNames string
	ServiceOfferingGUIDs string
}

func (l *ServicePlanList) ToMessage() repositories.ListServicePlanMessage {
	return repositories.ListServicePlanMessage{
		Names:                ParseArrayParam(l.Names),
		ServiceOfferingNames: ParseArrayParam(l.ServiceOfferingNames),
		ServiceOfferingGUIDs: ParseArrayParam(l.ServiceOfferingGUIDs),
	}
}

func (l *ServicePlanList) SupportedKeys() []string {
	return []string{"names", "service_offering_names", "page", "service_offering_guids"}
}

func (l *ServicePlanList) DecodeFromURLValues(values url.Values) error {
	l.Names = values.Get("names")
	l.ServiceOfferingNames = values.Get("service_offering_names")
	return nil
}
