package presenter

import (
	"net/url"

	"code.cloudfoundry.org/cf-k8s-controllers/api/repositories"
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
	if serviceInstanceRecord.CreatedAt == serviceInstanceRecord.UpdatedAt {
		lastOperationType = "create"
	}

	tags := serviceInstanceRecord.Tags
	if tags == nil {
		tags = []string{}
	}

	return ServiceInstanceResponse{
		Name: serviceInstanceRecord.Name,
		GUID: serviceInstanceRecord.GUID,
		Type: serviceInstanceRecord.Type,
		Tags: tags,
		LastOperation: lastOperation{
			CreatedAt:   serviceInstanceRecord.CreatedAt,
			UpdatedAt:   serviceInstanceRecord.UpdatedAt,
			Description: "Operation succeeded",
			State:       "succeeded",
			Type:        lastOperationType,
		},
		CreatedAt: serviceInstanceRecord.CreatedAt,
		UpdatedAt: serviceInstanceRecord.UpdatedAt,
		Relationships: Relationships{
			"space": Relationship{
				Data: &RelationshipData{
					GUID: serviceInstanceRecord.SpaceGUID,
				},
			},
		},
		Metadata: Metadata{
			Labels:      map[string]string{},
			Annotations: map[string]string{},
		},
		Links: ServiceInstanceLinks{
			Self: Link{
				HREF: buildURL(baseURL).appendPath(serviceInstancesBase, serviceInstanceRecord.GUID).build(),
			},
			Space: Link{
				HREF: buildURL(baseURL).appendPath(spacesBase, serviceInstanceRecord.SpaceGUID).build(),
			},
			Credentials: Link{
				HREF: buildURL(baseURL).appendPath(serviceInstancesBase, serviceInstanceRecord.GUID, "credentials").build(),
			},
			ServiceCredentialBindings: Link{
				HREF: buildURL(baseURL).appendPath(serviceCredentialBindingsBase).setQuery("service_instance_guids=" + serviceInstanceRecord.GUID).build(),
			},
			ServiceRouteBindings: Link{
				HREF: buildURL(baseURL).appendPath(serviceRouteBindingsBase).setQuery("service_instance_guids=" + serviceInstanceRecord.GUID).build(),
			},
		},
	}
}

func ForServiceInstanceList(serviceInstanceRecord []repositories.ServiceInstanceRecord, baseURL, requestURL url.URL) ListResponse {
	serviceInstanceResponses := make([]interface{}, 0, len(serviceInstanceRecord))
	for _, serviceInstance := range serviceInstanceRecord {
		serviceInstanceResponses = append(serviceInstanceResponses, ForServiceInstance(serviceInstance, baseURL))
	}

	return ForList(serviceInstanceResponses, baseURL, requestURL)
}
