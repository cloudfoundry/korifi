package presenter

import (
	"net/url"

	"code.cloudfoundry.org/korifi/api/repositories"
	"code.cloudfoundry.org/korifi/api/repositories/include"
	"code.cloudfoundry.org/korifi/tools"
)

const (
	serviceInstancesBase     = "/v3/service_instances"
	serviceRouteBindingsBase = "/v3/service_route_bindings"
	servicePlanBase          = "/v3/service_plans"
)

type ServiceInstanceResponse struct {
	Name            string        `json:"name"`
	GUID            string        `json:"guid"`
	Type            string        `json:"type"`
	Tags            []string      `json:"tags"`
	LastOperation   lastOperation `json:"last_operation"`
	RouteServiceURL *string       `json:"route_service_url"`
	SyslogDrainURL  *string       `json:"syslog_drain_url"`

	CreatedAt        string                       `json:"created_at"`
	UpdatedAt        string                       `json:"updated_at"`
	Relationships    map[string]ToOneRelationship `json:"relationships"`
	Metadata         Metadata                     `json:"metadata"`
	Links            ServiceInstanceLinks         `json:"links"`
	Included         map[string][]any             `json:"included,omitempty"`
	MaintenanceInfo  *MaintenanceInfo             `json:"maintenance_info,omitempty"`
	UpgradeAvailable *bool                        `json:"upgrade_available,omitempty"`
}

type lastOperation struct {
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
	Description string `json:"description"`
	State       string `json:"state"`
	Type        string `json:"type"`
}

type ServiceInstanceLinks struct {
	Self                      Link `json:"self"`
	Space                     Link `json:"space"`
	Credentials               Link `json:"credentials"`
	ServicePlan               Link `json:"service_plan"`
	ServiceCredentialBindings Link `json:"service_credential_bindings"`
	ServiceRouteBindings      Link `json:"service_route_bindings"`
}

func ForServiceInstance(serviceInstanceRecord repositories.ServiceInstanceRecord, baseURL url.URL, includes ...include.Resource) ServiceInstanceResponse {
	response := ServiceInstanceResponse{
		Name: serviceInstanceRecord.Name,
		GUID: serviceInstanceRecord.GUID,
		Type: serviceInstanceRecord.Type,
		Tags: emptySliceIfNil(serviceInstanceRecord.Tags),
		LastOperation: lastOperation{
			CreatedAt:   tools.ZeroIfNil(formatTimestamp(&serviceInstanceRecord.CreatedAt)),
			UpdatedAt:   tools.ZeroIfNil(formatTimestamp(serviceInstanceRecord.UpdatedAt)),
			Description: serviceInstanceRecord.LastOperation.Description,
			State:       serviceInstanceRecord.LastOperation.State,
			Type:        serviceInstanceRecord.LastOperation.Type,
		},
		CreatedAt:     tools.ZeroIfNil(formatTimestamp(&serviceInstanceRecord.CreatedAt)),
		UpdatedAt:     tools.ZeroIfNil(formatTimestamp(serviceInstanceRecord.UpdatedAt)),
		Relationships: ForRelationships(serviceInstanceRecord.Relationships()),
		Metadata: Metadata{
			Labels:      emptyMapIfNil(serviceInstanceRecord.Labels),
			Annotations: emptyMapIfNil(serviceInstanceRecord.Annotations),
		},
		Links: ServiceInstanceLinks{
			Self: Link{
				HRef: buildURL(baseURL).appendPath(serviceInstancesBase, serviceInstanceRecord.GUID).build(),
			},
			Space: Link{
				HRef: buildURL(baseURL).appendPath(spacesBase, serviceInstanceRecord.SpaceGUID).build(),
			},
			Credentials: Link{
				HRef: buildURL(baseURL).appendPath(serviceInstancesBase, serviceInstanceRecord.GUID, "credentials").build(),
			},
			ServicePlan: Link{
				HRef: buildURL(baseURL).appendPath(servicePlanBase, serviceInstanceRecord.PlanGUID).build(),
			},
			ServiceCredentialBindings: Link{
				HRef: buildURL(baseURL).appendPath(serviceCredentialBindingsBase).setQuery("service_instance_guids=" + serviceInstanceRecord.GUID).build(),
			},
			ServiceRouteBindings: Link{
				HRef: buildURL(baseURL).appendPath(serviceRouteBindingsBase).setQuery("service_instance_guids=" + serviceInstanceRecord.GUID).build(),
			},
		},
		Included: includedResources(includes...),
	}

	if serviceInstanceRecord.Type == "managed" {
		response.MaintenanceInfo = tools.PtrTo(MaintenanceInfo{Version: serviceInstanceRecord.MaintenanceInfo.Version})
		response.UpgradeAvailable = tools.PtrTo(serviceInstanceRecord.UpgradeAvailable)
	}

	return response
}
