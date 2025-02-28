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
	model.CFResource
	Name            string                             `json:"name"`
	Free            bool                               `json:"free"`
	Description     string                             `json:"description,omitempty"`
	BrokerCatalog   ServicePlanBrokerCatalog           `json:"broker_catalog"`
	Schemas         ServicePlanSchemas                 `json:"schemas"`
	MaintenanceInfo MaintenanceInfo                    `json:"maintenance_info"`
	VisibilityType  string                             `json:"visibility_type"`
	Available       bool                               `json:"available"`
	Relationships   map[string]model.ToOneRelationship `json:"relationships"`
	Links           ServicePlanLinks                   `json:"links"`
}

type ServicePlanBrokerCatalog struct {
	ID       string              `json:"id"`
	Metadata map[string]any      `json:"metadata,omitempty"`
	Features ServicePlanFeatures `json:"features"`
}

type InputParameterSchema struct {
	Parameters map[string]any `json:"parameters,omitempty"`
}

type ServiceInstanceSchema struct {
	Create InputParameterSchema `json:"create"`
	Update InputParameterSchema `json:"update"`
}

type ServiceBindingSchema struct {
	Create InputParameterSchema `json:"create"`
}

type ServicePlanSchemas struct {
	ServiceInstance ServiceInstanceSchema `json:"service_instance"`
	ServiceBinding  ServiceBindingSchema  `json:"service_binding"`
}

type ServicePlanFeatures struct {
	PlanUpdateable bool `json:"plan_updateable"`
	Bindable       bool `json:"bindable"`
}

type MaintenanceInfo struct {
	Version string `json:"version"`
}

func ForServicePlan(servicePlan repositories.ServicePlanRecord, baseURL url.URL, includes ...model.IncludedResource) ServicePlanResponse {
	return ServicePlanResponse{
		Name:        servicePlan.Name,
		Free:        servicePlan.Free,
		Description: servicePlan.Description,
		BrokerCatalog: ServicePlanBrokerCatalog{
			ID:       servicePlan.BrokerCatalog.ID,
			Metadata: servicePlan.BrokerCatalog.Metadata,
			Features: ServicePlanFeatures{
				PlanUpdateable: servicePlan.BrokerCatalog.Features.PlanUpdateable,
				Bindable:       servicePlan.BrokerCatalog.Features.Bindable,
			},
		},
		CFResource:      servicePlan.CFResource,
		VisibilityType:  servicePlan.Visibility.Type,
		MaintenanceInfo: MaintenanceInfo(servicePlan.MaintenanceInfo),
		Schemas: ServicePlanSchemas{
			ServiceInstance: ServiceInstanceSchema{
				Create: InputParameterSchema(servicePlan.Schemas.ServiceInstance.Create),
				Update: InputParameterSchema(servicePlan.Schemas.ServiceInstance.Update),
			},
			ServiceBinding: ServiceBindingSchema{
				Create: InputParameterSchema(servicePlan.Schemas.ServiceBinding.Create),
			},
		},
		Available:     servicePlan.Available,
		Relationships: ForRelationships(servicePlan.Relationships()),
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
