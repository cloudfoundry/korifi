package presenter

import (
	"net/url"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
)

const (
	serviceOfferingsBase = "/v3/service_offerings"
	servicePlansBase     = "/v3/service_plans"
)

type ServiceOfferingLinks struct {
	Self         Link `json:"self"`
	ServicePlans Link `json:"service_plans"`
	Visibility   Link `json:"visibility"`
}

type ServicePlanLinks struct {
	Self            Link `json:"self"`
	ServiceOffering Link `json:"service_offering"`
	Visibility      Link `json:"visibility"`
}

type ServiceOfferingResponse struct {
	korifiv1alpha1.ServiceOfferingResource
	Links ServiceOfferingLinks `json:"links"`
}

type ServicePlanResponse struct {
	korifiv1alpha1.ServicePlanResource
	Links ServicePlanLinks `json:"links"`
}

type ServicePlanVisibilityResponse struct {
	Type          string                   `json:"type"`
	Organizations []VisibilityOrganization `json:"organizations,omitempty"`
}

type VisibilityOrganization struct {
	GUID string `json:"guid"`
	Name string `json:"name"`
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

func ForServiceOffering(serviceOfferingResource korifiv1alpha1.ServiceOfferingResource, baseURL url.URL) ServiceOfferingResponse {
	return ServiceOfferingResponse{
		ServiceOfferingResource: serviceOfferingResource,
		Links: ServiceOfferingLinks{
			Self: Link{
				HRef: buildURL(baseURL).appendPath(serviceOfferingsBase, serviceOfferingResource.GUID).build(),
			},
			ServicePlans: Link{
				HRef: buildURL(baseURL).appendPath(servicePlansBase).setQuery("service_offering_guids=" + serviceOfferingResource.GUID).build(),
			},
			Visibility: Link{
				HRef: buildURL(baseURL).appendPath(servicePlansBase, serviceOfferingResource.GUID, "visibility").build(),
			},
		},
	}
}

func ForServiceOfferingList(serviceOfferingResourceList []korifiv1alpha1.ServiceOfferingResource, baseURL, requestURL url.URL) ListResponse[ServiceOfferingResponse] {
	return ForList(func(serviceOfferingResource korifiv1alpha1.ServiceOfferingResource, baseURL url.URL) ServiceOfferingResponse {
		return ForServiceOffering(serviceOfferingResource, baseURL)
	}, serviceOfferingResourceList, baseURL, requestURL)
}

func ForServicePlan(servicePlanResource korifiv1alpha1.ServicePlanResource, baseURL url.URL) ServicePlanResponse {
	return ServicePlanResponse{
		ServicePlanResource: servicePlanResource,
		Links: ServicePlanLinks{
			Self: Link{
				HRef: buildURL(baseURL).appendPath(servicePlansBase, servicePlanResource.GUID).build(),
			},
			ServiceOffering: Link{
				HRef: buildURL(baseURL).appendPath(serviceOfferingsBase, servicePlanResource.Relationships.Service_offering.Data.GUID).build(),
			},
			Visibility: Link{
				HRef: buildURL(baseURL).appendPath(servicePlansBase, servicePlanResource.GUID, "visibility").build(),
			},
		},
	}
}

func ForServicePlanList(servicePlanResourceList []korifiv1alpha1.ServicePlanResource, baseURL, requestURL url.URL, includes ...IncludedResources) ListResponse[ServicePlanResponse] {
	return ForList(func(servicePlanResource korifiv1alpha1.ServicePlanResource, baseURL url.URL) ServicePlanResponse {
		return ForServicePlan(servicePlanResource, baseURL)
	}, servicePlanResourceList, baseURL, requestURL, includes...)
}

func ForServicePlanVisibility(servicePlanVisibilityResource korifiv1alpha1.ServicePlanVisibilityResource) ServicePlanVisibilityResponse {
	orgs := []VisibilityOrganization{}
	for _, org := range servicePlanVisibilityResource.Organizations {
		orgs = append(orgs, VisibilityOrganization{
			GUID: org.GUID,
			Name: org.Name,
		})
	}
	return ServicePlanVisibilityResponse{
		Type:          servicePlanVisibilityResource.Type,
		Organizations: orgs,
	}
}
