package payloads

import (
	"net/url"
	"regexp"

	"code.cloudfoundry.org/korifi/api/payloads/parse"
	"code.cloudfoundry.org/korifi/api/repositories"
)

type ServicePlanList struct {
	ServiceOfferingGUIDs string
}

func (l *ServicePlanList) ToMessage() repositories.ListServicePlanMessage {
	return repositories.ListServicePlanMessage{
		ServiceOfferingGUIDs: parse.ArrayParam(l.ServiceOfferingGUIDs),
	}
}

func (l *ServicePlanList) SupportedKeys() []string {
	return []string{"service_offering_guids", "page", "per_page", "include"}
}

func (l *ServicePlanList) IgnoredKeys() []*regexp.Regexp {
	return []*regexp.Regexp{regexp.MustCompile(`fields\[.+\]`)}
}

func (l *ServicePlanList) DecodeFromURLValues(values url.Values) error {
	l.ServiceOfferingGUIDs = values.Get("service_offering_guids")
	return nil
}

type ServicePlanVisibility struct {
	Type string `json:"type"`
}

func (p *ServicePlanVisibility) ToMessage(planGUID string) repositories.ApplyServicePlanVisibilityMessage {
	return repositories.ApplyServicePlanVisibilityMessage{
		PlanGUID: planGUID,
		Type:     p.Type,
	}
}
