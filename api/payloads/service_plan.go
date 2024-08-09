package payloads

import (
	"fmt"
	"net/url"
	"regexp"
	"strconv"

	"code.cloudfoundry.org/korifi/api/payloads/parse"
	"code.cloudfoundry.org/korifi/api/payloads/validation"
	"code.cloudfoundry.org/korifi/api/repositories"
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/model/services"
	"code.cloudfoundry.org/korifi/tools"
	"github.com/BooleanCat/go-functional/iter"
	jellidation "github.com/jellydator/validation"
)

type ServicePlanList struct {
	ServiceOfferingGUIDs string
	Names                string
	Available            *bool
	IncludeResources     []string
}

func (l ServicePlanList) Validate() error {
	return jellidation.ValidateStruct(&l,
		jellidation.Field(&l.IncludeResources, jellidation.Each(validation.OneOf("service_offering"))),
	)
}

func (l *ServicePlanList) ToMessage() repositories.ListServicePlanMessage {
	return repositories.ListServicePlanMessage{
		ServiceOfferingGUIDs: parse.ArrayParam(l.ServiceOfferingGUIDs),
		Names:                parse.ArrayParam(l.Names),
		Available:            l.Available,
	}
}

func (l *ServicePlanList) SupportedKeys() []string {
	return []string{"service_offering_guids", "names", "available", "page", "per_page", "include"}
}

func (l *ServicePlanList) IgnoredKeys() []*regexp.Regexp {
	return []*regexp.Regexp{regexp.MustCompile(`fields\[.+\]`)}
}

func (l *ServicePlanList) DecodeFromURLValues(values url.Values) error {
	l.ServiceOfferingGUIDs = values.Get("service_offering_guids")
	l.Names = values.Get("names")

	available, err := parseBool(values.Get("available"))
	if err != nil {
		return fmt.Errorf("failed to parse 'available' query parameter: %w", err)
	}
	l.Available = available
	l.IncludeResources = parse.ArrayParam(values.Get("include"))

	return nil
}

func parseBool(valueStr string) (*bool, error) {
	if valueStr == "" {
		return nil, nil
	}

	valueBool, err := strconv.ParseBool(valueStr)
	if err != nil {
		return nil, err
	}
	return tools.PtrTo(valueBool), nil
}

type ServicePlanVisibility struct {
	Type          string                            `json:"type"`
	Organizations []services.VisibilityOrganization `json:"organizations"`
}

func (p ServicePlanVisibility) Validate() error {
	organizationsRule := jellidation.By(func(value any) error {
		orgs, ok := value.([]services.VisibilityOrganization)
		if !ok {
			return fmt.Errorf("%T is not supported, []services.VisibilityOrganization is expected", value)
		}

		if p.Type != korifiv1alpha1.OrganizationServicePlanVisibilityType {
			return jellidation.Empty.Validate(orgs)
		}

		return jellidation.Required.Validate(orgs)
	})

	return jellidation.ValidateStruct(&p,
		jellidation.Field(&p.Type, validation.OneOf(
			korifiv1alpha1.AdminServicePlanVisibilityType,
			korifiv1alpha1.PublicServicePlanVisibilityType,
			korifiv1alpha1.OrganizationServicePlanVisibilityType,
		)),
		jellidation.Field(&p.Organizations, organizationsRule),
	)
}

func (p *ServicePlanVisibility) ToApplyMessage(planGUID string) repositories.ApplyServicePlanVisibilityMessage {
	return repositories.ApplyServicePlanVisibilityMessage{
		PlanGUID: planGUID,
		Type:     p.Type,
		Organizations: iter.Map(iter.Lift(p.Organizations), func(v services.VisibilityOrganization) string {
			return v.GUID
		}).Collect(),
	}
}

func (p *ServicePlanVisibility) ToUpdateMessage(planGUID string) repositories.UpdateServicePlanVisibilityMessage {
	return repositories.UpdateServicePlanVisibilityMessage{
		PlanGUID: planGUID,
		Type:     p.Type,
		Organizations: iter.Map(iter.Lift(p.Organizations), func(v services.VisibilityOrganization) string {
			return v.GUID
		}).Collect(),
	}
}
