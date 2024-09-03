package presenter

import (
	"net/url"

	"code.cloudfoundry.org/korifi/api/repositories"
	"code.cloudfoundry.org/korifi/model"
	"code.cloudfoundry.org/korifi/model/services"
)

type ServicePlanLinks struct {
	Self            Link `json:"self"`
	ServiceOffering Link `json:"service_offering"`
	Visibility      Link `json:"visibility"`
}

type ServicePlanRelationships struct {
	ServiceOffering model.ToOneRelationship `json:"service_offering"`
}

type ServicePlanResponse struct {
	services.ServicePlan
	model.CFResource
	VisibilityType string                             `json:"visibility_type"`
	Available      bool                               `json:"available"`
	Relationships  map[string]model.ToOneRelationship `json:"relationships"`
	Links          ServicePlanLinks                   `json:"links"`
}

func ForServicePlan(servicePlan repositories.ServicePlanRecord, baseURL url.URL) ServicePlanResponse {
	return ServicePlanResponse{
		ServicePlan:    servicePlan.ServicePlan,
		CFResource:     servicePlan.CFResource,
		VisibilityType: servicePlan.Visibility.Type,
		Available:      servicePlan.Available,
		Relationships:  ForRelationships(servicePlan.Relationships()),
		Links: ServicePlanLinks{
			Self: Link{
				HRef: buildURL(baseURL).appendPath(servicePlansBase, servicePlan.GUID).build(),
			},
			ServiceOffering: Link{
				HRef: buildURL(baseURL).appendPath(serviceOfferingsBase, servicePlan.ServiceOfferingGUID).build(),
			},
			Visibility: Link{
				HRef: buildURL(baseURL).appendPath(servicePlansBase, servicePlan.GUID, "visibility").build(),
			},
		},
	}
}

type ServicePlanVisibilityResponse struct {
	Type          string                            `json:"type"`
	Organizations []services.VisibilityOrganization `json:"organizations,omitempty"`
}

func ForServicePlanVisibility(plan repositories.ServicePlanRecord, _ url.URL) ServicePlanVisibilityResponse {
	return ServicePlanVisibilityResponse{
		Type:          plan.Visibility.Type,
		Organizations: plan.Visibility.Organizations,
	}
}
