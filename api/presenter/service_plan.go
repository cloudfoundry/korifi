package presenter

import (
	"net/url"
	"slices"
	"time"

	"code.cloudfoundry.org/korifi/api/repositories"
	"code.cloudfoundry.org/korifi/api/repositories/include"
	"github.com/BooleanCat/go-functional/v2/it"
)

type ServicePlanLinks struct {
	Self            Link `json:"self"`
	ServiceOffering Link `json:"service_offering"`
	Visibility      Link `json:"visibility"`
}

type ServicePlanRelationships struct {
	ServiceOffering ToOneRelationship `json:"service_offering"`
}

type ServicePlanResponse struct {
	Name            string                       `json:"name"`
	GUID            string                       `json:"guid"`
	CreatedAt       time.Time                    `json:"created_at"`
	UpdatedAt       *time.Time                   `json:"updated_at"`
	Metadata        Metadata                     `json:"metadata"`
	Free            bool                         `json:"free"`
	Description     string                       `json:"description,omitempty"`
	BrokerCatalog   ServicePlanBrokerCatalog     `json:"broker_catalog"`
	Schemas         ServicePlanSchemas           `json:"schemas"`
	MaintenanceInfo MaintenanceInfo              `json:"maintenance_info"`
	VisibilityType  string                       `json:"visibility_type"`
	Available       bool                         `json:"available"`
	Relationships   map[string]ToOneRelationship `json:"relationships"`
	Links           ServicePlanLinks             `json:"links"`
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

func ForServicePlan(servicePlan repositories.ServicePlanRecord, baseURL url.URL, includes ...include.Resource) ServicePlanResponse {
	return ServicePlanResponse{
		Name:      servicePlan.Name,
		GUID:      servicePlan.GUID,
		CreatedAt: servicePlan.CreatedAt,
		UpdatedAt: servicePlan.UpdatedAt,
		Metadata: Metadata{
			Labels:      servicePlan.Metadata.Labels,
			Annotations: servicePlan.Metadata.Annotations,
		},
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
	Type          string                   `json:"type"`
	Organizations []VisibilityOrganization `json:"organizations,omitempty"`
}

type VisibilityOrganization struct {
	GUID string `json:"guid"`
	Name string `json:"name"`
}

func ForServicePlanVisibility(plan repositories.ServicePlanRecord, _ url.URL) ServicePlanVisibilityResponse {
	return ServicePlanVisibilityResponse{
		Type: plan.Visibility.Type,
		Organizations: slices.Collect(
			it.Map(slices.Values(plan.Visibility.Organizations),
				func(o repositories.VisibilityOrganization) VisibilityOrganization {
					return VisibilityOrganization(o)
				},
			),
		),
	}
}
