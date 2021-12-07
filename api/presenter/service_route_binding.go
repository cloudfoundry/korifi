package presenter

import "net/url"

const (
	serviceRouteBindingsBase = "/v3/service_route_bindings"
)

type ServiceRouteBinding struct{}

type ServiceRouteBindingsResponse struct {
	PaginationData PaginationData        `json:"pagination"`
	Resources      []ServiceRouteBinding `json:"resources"`
}

func ForServiceRouteBindingsList(baseURL url.URL) ServiceRouteBindingsResponse {
	return ServiceRouteBindingsResponse{
		PaginationData: PaginationData{
			TotalResults: 0,
			TotalPages:   1,
			First: PageRef{
				HREF: buildURL(baseURL).appendPath(serviceRouteBindingsBase).build(),
			},
			Last: PageRef{
				HREF: buildURL(baseURL).appendPath(serviceRouteBindingsBase).build(),
			},
		},
		Resources: []ServiceRouteBinding{},
	}
}
