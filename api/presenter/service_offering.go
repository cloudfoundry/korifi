package presenter

import (
	"net/url"

	"code.cloudfoundry.org/korifi/api/repositories"
)

const (
	serviceOfferingsBase = "/v3/service_offerings"
	servicePlansBase     = "/v3/service_plans"
	serviceBrokersBase   = "/v3/service_brokers"
)

type ServiceOfferingResponse struct {
	Name             string               `json:"name"`
	GUID             string               `json:"guid"`
	Description      string               `json:"description"`
	Available        bool                 `json:"available"`
	Tags             []string             `json:"tags"`
	Requires         []string             `json:"requires"`
	Shareable        bool                 `json:"shareable"`
	DocumentationUrl string               `json:"documentation_url"`
	BrokerCatalog    BrokerCatalog        `json:"broker_catalog"`
	CreatedAt        string               `json:"created_at"`
	UpdatedAt        string               `json:"updated_at"`
	Relationships    Relationships        `json:"relationships"`
	Metadata         Metadata             `json:"metadata"`
	Links            ServiceOfferingLinks `json:"links"`
}

type ServiceOfferingLinks struct {
	Self         Link `json:"self"`
	ServicePlans Link `json:"service_plans"`
	Visibility   Link `json:"visibility"`
}

type BrokerCatalog struct {
	Id       string                `json:"id"`
	Metadata BrokerCatalogMetadata `json:"metadata"`
	Features BrokerCatalogFeatures `json:"features"`
}

type BrokerCatalogMetadata struct {
	Shareable bool `json:"shareable"`
}

type BrokerCatalogFeatures struct {
	PlanUpdateable       bool `json:"plan_updateable"`
	Bindable             bool `json:"bindable"`
	InstancesRetrievable bool `json:"instances_retrievable"`
	BindingsRetrievable  bool `json:"bindings_retrievable"`
	AllowContextUpdates  bool `json:"allow_context_updates"`
}

func ForServiceOffering(serviceOfferingRecord repositories.ServiceOfferingRecord, baseURL url.URL) ServiceOfferingResponse {
	return ServiceOfferingResponse{
		Name:             serviceOfferingRecord.Name,
		GUID:             serviceOfferingRecord.GUID,
		Description:      serviceOfferingRecord.Description,
		Available:        serviceOfferingRecord.Available,
		Tags:             serviceOfferingRecord.Tags,
		Requires:         serviceOfferingRecord.Requires,
		Shareable:        serviceOfferingRecord.Shareable,
		DocumentationUrl: serviceOfferingRecord.DocumentationUrl,
		BrokerCatalog: BrokerCatalog{
			Id: serviceOfferingRecord.CatalogId,
			Metadata: BrokerCatalogMetadata{
				Shareable: serviceOfferingRecord.Shareable,
			},
			Features: BrokerCatalogFeatures{
				PlanUpdateable:       serviceOfferingRecord.PlanUpdateable,
				Bindable:             serviceOfferingRecord.Bindable,
				InstancesRetrievable: serviceOfferingRecord.InstancesRetrievable,
				BindingsRetrievable:  serviceOfferingRecord.BindingsRetrievable,
				AllowContextUpdates:  serviceOfferingRecord.AllowContextUpdates,
			},
		},
		CreatedAt: serviceOfferingRecord.CreatedAt,
		UpdatedAt: serviceOfferingRecord.UpdatedAt,
		Relationships: Relationships{
			"service_broker": Relationship{
				Data: &RelationshipData{
					GUID: serviceOfferingRecord.BrokerId,
				},
			},
		},
		Metadata: Metadata{
			Labels:      map[string]string{},
			Annotations: map[string]string{},
		},
		Links: ServiceOfferingLinks{
			Self: Link{
				HRef: buildURL(baseURL).appendPath(serviceOfferingsBase, serviceOfferingRecord.GUID).build(),
			},
			ServicePlans: Link{
				HRef: buildURL(baseURL).appendPath(servicePlansBase).setQuery("service_offering_guids=" + serviceOfferingRecord.GUID).build(),
			},
			Visibility: Link{
				HRef: buildURL(baseURL).appendPath(serviceBrokersBase, serviceOfferingRecord.BrokerId).build(),
			},
		},
	}
}

func ForServiceOfferingList(serviceInstanceRecord []repositories.ServiceOfferingRecord, baseURL, requestURL url.URL) ListResponse {
	serviceInstanceResponses := make([]interface{}, 0, len(serviceInstanceRecord))
	for _, serviceInstance := range serviceInstanceRecord {
		serviceInstanceResponses = append(serviceInstanceResponses, ForServiceOffering(serviceInstance, baseURL))
	}

	return ForList(serviceInstanceResponses, baseURL, requestURL)
}

type ServicePlanResponse struct {
	Name           string           `json:"name"`
	GUID           string           `json:"guid"`
	Description    string           `json:"description"`
	Available      bool             `json:"available"`
	CreatedAt      string           `json:"created_at"`
	UpdatedAt      string           `json:"updated_at"`
	Relationships  Relationships    `json:"relationships"`
	Metadata       Metadata         `json:"metadata"`
	Links          ServicePlanLinks `json:"links"`
	VisibilityType string           `json:"visibility_type"`
	Free           bool             `json:"free"`
	Costs          []struct{}       `json:"costs"`
	MaitenanceInfo struct{}         `json:"maitenance_info"`
	BrokerCatalog  BrokerCatalog    `json:"broker_catalog"`
	Schemas        map[string]any   `json:"schemas"`
}

type ServicePlanLinks struct {
	Self            Link `json:"self"`
	ServiceOffering Link `json:"service_offering"`
	Visibility      Link `json:"visibility"`
}

func ForServicePlan(servicePlanRecord repositories.ServicePlanRecord, baseURL url.URL) ServicePlanResponse {
	return ServicePlanResponse{
		Name:        servicePlanRecord.Name,
		GUID:        servicePlanRecord.GUID,
		Description: servicePlanRecord.Description,
		Available:   servicePlanRecord.Available,
		BrokerCatalog: BrokerCatalog{
			Id: servicePlanRecord.CatalogId,
			Features: BrokerCatalogFeatures{
				PlanUpdateable: servicePlanRecord.PlanUpdateable,
				Bindable:       servicePlanRecord.Bindable,
			},
		},
		CreatedAt:      servicePlanRecord.CreatedAt,
		UpdatedAt:      servicePlanRecord.UpdatedAt,
		VisibilityType: servicePlanRecord.VisibilityType,
		Free:           servicePlanRecord.Free,
		Schemas:        servicePlanRecord.Schemas,
		Relationships: Relationships{
			"service_offering": Relationship{
				Data: &RelationshipData{
					GUID: servicePlanRecord.ServiceOfferingGUID,
				},
			},
		},
		Metadata: Metadata{
			Labels:      map[string]string{},
			Annotations: map[string]string{},
		},
		Links: ServicePlanLinks{
			Self: Link{
				HRef: buildURL(baseURL).appendPath(servicePlansBase, servicePlanRecord.GUID).build(),
			},
			ServiceOffering: Link{
				HRef: buildURL(baseURL).appendPath(serviceOfferingsBase, servicePlanRecord.ServiceOfferingGUID).build(),
			},
			Visibility: Link{
				HRef: buildURL(baseURL).appendPath(serviceBrokersBase, servicePlanRecord.GUID, "visibility").build(),
			},
		},
	}
}

func ForServicePlanList(serviceInstanceRecord []repositories.ServicePlanRecord, baseURL, requestURL url.URL) ListResponse {
	serviceInstanceResponses := make([]interface{}, 0, len(serviceInstanceRecord))
	for _, serviceInstance := range serviceInstanceRecord {
		serviceInstanceResponses = append(serviceInstanceResponses, ForServicePlan(serviceInstance, baseURL))
	}

	return ForList(serviceInstanceResponses, baseURL, requestURL)
}
