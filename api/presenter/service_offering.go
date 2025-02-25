package presenter

import (
	"net/url"

	"code.cloudfoundry.org/korifi/api/repositories"
	"code.cloudfoundry.org/korifi/model"
)

const (
	serviceOfferingsBase = "/v3/service_offerings"
	servicePlansBase     = "/v3/service_plans"
)

type ServiceOfferingLinks struct {
	Self          Link `json:"self"`
	ServicePlans  Link `json:"service_plans"`
	ServiceBroker Link `json:"service_broker"`
}

type ServiceOfferingResponse struct {
	Name             string               `json:"name"`
	Description      string               `json:"description"`
	Tags             []string             `json:"tags,omitempty"`
	Requires         []string             `json:"requires,omitempty"`
	DocumentationURL *string              `json:"documentation_url"`
	BrokerCatalog    ServiceBrokerCatalog `json:"broker_catalog"`
	model.CFResource
	Relationships ServiceOfferingRelationships `json:"relationships"`
	Links         ServiceOfferingLinks         `json:"links"`
	Included      map[string][]any             `json:"included,omitempty"`
}

type ServiceBrokerCatalog struct {
	ID       string                `json:"id"`
	Metadata map[string]any        `json:"metadata"`
	Features BrokerCatalogFeatures `json:"features"`
}

type BrokerCatalogFeatures struct {
	PlanUpdateable       bool `json:"plan_updateable"`
	Bindable             bool `json:"bindable"`
	InstancesRetrievable bool `json:"instances_retrievable"`
	BindingsRetrievable  bool `json:"bindings_retrievable"`
	AllowContextUpdates  bool `json:"allow_context_updates"`
}

type ServiceOfferingRelationships struct {
	ServiceBroker model.ToOneRelationship `json:"service_broker"`
}

func ForServiceOffering(serviceOffering repositories.ServiceOfferingRecord, baseURL url.URL, includes ...model.IncludedResource) ServiceOfferingResponse {
	return ServiceOfferingResponse{
		Name:             serviceOffering.Name,
		Description:      serviceOffering.Description,
		Tags:             serviceOffering.Tags,
		Requires:         serviceOffering.Requires,
		DocumentationURL: serviceOffering.DocumentationURL,
		BrokerCatalog: ServiceBrokerCatalog{
			ID:       serviceOffering.BrokerCatalog.ID,
			Metadata: serviceOffering.BrokerCatalog.Metadata,
			Features: BrokerCatalogFeatures{
				PlanUpdateable:       serviceOffering.BrokerCatalog.Features.PlanUpdateable,
				Bindable:             serviceOffering.BrokerCatalog.Features.Bindable,
				InstancesRetrievable: serviceOffering.BrokerCatalog.Features.InstancesRetrievable,
				BindingsRetrievable:  serviceOffering.BrokerCatalog.Features.BindingsRetrievable,
				AllowContextUpdates:  serviceOffering.BrokerCatalog.Features.AllowContextUpdates,
			},
		},
		CFResource: serviceOffering.CFResource,
		Relationships: ServiceOfferingRelationships{
			ServiceBroker: model.ToOneRelationship{
				Data: model.Relationship{
					GUID: serviceOffering.ServiceBrokerGUID,
				},
			},
		},
		Links: ServiceOfferingLinks{
			Self: Link{
				HRef: buildURL(baseURL).appendPath(serviceOfferingsBase, serviceOffering.GUID).build(),
			},
			ServicePlans: Link{
				HRef: buildURL(baseURL).appendPath(servicePlansBase).setQuery("service_offering_guids=" + serviceOffering.GUID).build(),
			},
			ServiceBroker: Link{
				HRef: buildURL(baseURL).appendPath(serviceBrokersBase, serviceOffering.ServiceBrokerGUID).build(),
			},
		},
		Included: includedResources(includes...),
	}
}
