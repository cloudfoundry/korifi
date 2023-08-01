package presenter

import (
	"net/url"

	"code.cloudfoundry.org/korifi/api/repositories"
)

const (
	serviceInstancesBase     = "/v3/service_instances"
	serviceRouteBindingsBase = "/v3/service_route_bindings"
)

type ServiceInstanceResponse struct {
	Name            string        `json:"name"`
	GUID            string        `json:"guid"`
	Type            string        `json:"type"`
	Tags            []string      `json:"tags"`
	LastOperation   lastOperation `json:"last_operation"`
	RouteServiceURL *string       `json:"route_service_url"`
	SyslogDrainURL  *string       `json:"syslog_drain_url"`

	CreatedAt     string               `json:"created_at"`
	UpdatedAt     string               `json:"updated_at"`
	Relationships Relationships        `json:"relationships"`
	Metadata      Metadata             `json:"metadata"`
	Links         ServiceInstanceLinks `json:"links"`
}

type ServiceInstanceParametersResponse map[string]any

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
	ServiceCredentialBindings Link `json:"service_credential_bindings"`
	ServiceRouteBindings      Link `json:"service_route_bindings"`
}

func ForServiceInstance(serviceInstanceRecord repositories.ServiceInstanceRecord, baseURL url.URL) ServiceInstanceResponse {
	lastOperationType := "update"
	if serviceInstanceRecord.UpdatedAt == nil || serviceInstanceRecord.CreatedAt == *serviceInstanceRecord.UpdatedAt {
		lastOperationType = "create"
	}

	return ServiceInstanceResponse{
		Name: serviceInstanceRecord.Name,
		GUID: serviceInstanceRecord.GUID,
		Type: serviceInstanceRecord.Type,
		Tags: emptySliceIfNil(serviceInstanceRecord.Tags),
		LastOperation: lastOperation{
			CreatedAt:   formatTimestamp(&serviceInstanceRecord.CreatedAt),
			UpdatedAt:   formatTimestamp(serviceInstanceRecord.UpdatedAt),
			Description: "Operation succeeded",
			State:       "succeeded",
			Type:        lastOperationType,
		},
		CreatedAt: formatTimestamp(&serviceInstanceRecord.CreatedAt),
		UpdatedAt: formatTimestamp(serviceInstanceRecord.UpdatedAt),
		Relationships: Relationships{
			"space": Relationship{
				Data: &RelationshipData{
					GUID: serviceInstanceRecord.SpaceGUID,
				},
			},
			"service_plan": Relationship{
				Data: &RelationshipData{
					GUID: serviceInstanceRecord.PlanGUID,
				},
			},
		},
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
			ServiceCredentialBindings: Link{
				HRef: buildURL(baseURL).appendPath(serviceCredentialBindingsBase).setQuery("service_instance_guids=" + serviceInstanceRecord.GUID).build(),
			},
			ServiceRouteBindings: Link{
				HRef: buildURL(baseURL).appendPath(serviceRouteBindingsBase).setQuery("service_instance_guids=" + serviceInstanceRecord.GUID).build(),
			},
		},
	}
}

func ForServiceInstanceList(serviceInstanceRecordList []repositories.ServiceInstanceRecord, baseURL, requestURL url.URL) ListResponse[ServiceInstanceResponse] {
	return ForList(func(serviceInstanceRecord repositories.ServiceInstanceRecord, baseURL url.URL) ServiceInstanceResponse {
		return ForServiceInstance(serviceInstanceRecord, baseURL)
	}, serviceInstanceRecordList, baseURL, requestURL)
}

func ForServiceInstanceParameters(serviceInstanceRecord repositories.ServiceInstanceRecord) ServiceInstanceParametersResponse {
	return serviceInstanceRecord.Parameters
}
