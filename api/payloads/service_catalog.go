package payloads

import (
	"net/url"
	"regexp"
	"strconv"

	"code.cloudfoundry.org/korifi/api/payloads/parse"
	"code.cloudfoundry.org/korifi/api/payloads/validation"
	"code.cloudfoundry.org/korifi/api/repositories"
	jellidation "github.com/jellydator/validation"
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
	return []string{"names", "page", "space_guids"}
}

func (l *ServiceOfferingList) IgnoredKeys() []*regexp.Regexp {
	return []*regexp.Regexp{regexp.MustCompile(`fields\[.+\]`)}
}

func (l *ServiceOfferingList) DecodeFromURLValues(values url.Values) error {
	l.Names = values.Get("names")
	return nil
}

type ServicePlanList struct {
	Names                string
	Available            *bool
	SpaceGuids           string
	ServiceOfferingNames string
	ServiceOfferingGUIDs string
	Include              []string
	ServiceBrokerDetails []string
}

func (l *ServicePlanList) ToMessage() repositories.ListServicePlanMessage {
	return repositories.ListServicePlanMessage{
		Names:                parse.ArrayParam(l.Names),
		Available:            l.Available,
		SpaceGuids:           parse.ArrayParam(l.SpaceGuids),
		ServiceOfferingNames: parse.ArrayParam(l.ServiceOfferingNames),
		ServiceOfferingGUIDs: parse.ArrayParam(l.ServiceOfferingGUIDs),
	}
}

func (l *ServicePlanList) SupportedKeys() []string {
	return []string{"names", "available", "broker_catalog_ids", "space_guids", "organization_guids", "service_broker_guids", "service_broker_names", "service_offering_names", "service_offering_guids", "include", "page", "per_page", "order_by", "label_selector", "fields", "fields[service_offering.service_broker]", "created_ats", "updated_ats"}
}

func (l *ServicePlanList) DecodeFromURLValues(values url.Values) error {
	var err error
	l.Names = values.Get("names")
	l.Available, err = parseBool(values.Get("available"))
	l.ServiceOfferingNames = values.Get("service_offering_names")
	l.ServiceOfferingGUIDs = values.Get("service_offering_guids")
	l.Include = parse.ArrayParam(values.Get("include"))
	l.ServiceBrokerDetails = parse.ArrayParam(values.Get("fields[service_offering.service_broker]"))

	return err
}

func (l ServicePlanList) Validate() error {
	return jellidation.ValidateStruct(&l,
		jellidation.Field(&l.Include,
			jellidation.Each(validation.OneOf("service_offering", "space.organization")),
		),
		jellidation.Field(&l.ServiceBrokerDetails,
			jellidation.Each(validation.OneOf("guid", "name")),
		),
	)
}

func parseBool(value string) (*bool, error) {
	if value != "" {
		parsed, err := strconv.ParseBool(value)
		return &parsed, err
	}
	return nil, nil
}

type PlanVisiblityApply struct {
	Type          string                   `json:"type"`
	Organizations []VisibilityOrganization `json:"organizations"`
}

type VisibilityOrganization struct {
	GUID string `json:"guid"`
}

func (v PlanVisiblityApply) Validate() error {
	return jellidation.ValidateStruct(&v,
		jellidation.Field(&v.Type,
			validation.OneOf("public", "admin", "organization"),
		),
	)
}

func (v *PlanVisiblityApply) ToMessage(planGUID string) repositories.PlanVisibilityApplyMessage {
	orgGUIDs := []string{}
	for _, org := range v.Organizations {
		orgGUIDs = append(orgGUIDs, org.GUID)
	}

	return repositories.PlanVisibilityApplyMessage{
		PlanGUID:      planGUID,
		Type:          v.Type,
		Organizations: orgGUIDs,
	}
}
