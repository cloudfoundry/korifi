package presenter

import (
	"fmt"
	"net/url"

	"code.cloudfoundry.org/korifi/api/repositories"
	"code.cloudfoundry.org/korifi/api/repositories/include"
	"code.cloudfoundry.org/korifi/tools"
)

const (
	routesBase = "/v3/routes"
)

type RouteResponse struct {
	GUID         string             `json:"guid"`
	Protocol     string             `json:"protocol"`
	Port         *int               `json:"port"`
	Host         string             `json:"host"`
	Path         string             `json:"path"`
	URL          string             `json:"url"`
	Destinations []routeDestination `json:"destinations"`

	CreatedAt     string                       `json:"created_at"`
	UpdatedAt     string                       `json:"updated_at"`
	Relationships map[string]ToOneRelationship `json:"relationships"`
	Metadata      Metadata                     `json:"metadata"`
	Links         routeLinks                   `json:"links"`
}

type RouteDestinationsResponse struct {
	Destinations []routeDestination     `json:"destinations"`
	Links        routeDestinationsLinks `json:"links"`
}

type routeDestination struct {
	GUID     string              `json:"guid"`
	App      routeDestinationApp `json:"app"`
	Weight   *int                `json:"weight"`
	Port     *int32              `json:"port"`
	Protocol *string             `json:"protocol"`
}

type routeDestinationApp struct {
	AppGUID string                     `json:"guid"`
	Process routeDestinationAppProcess `json:"process"`
}

type routeDestinationAppProcess struct {
	Type string `json:"type"`
}

type routeLinks struct {
	Self         Link `json:"self"`
	Space        Link `json:"space"`
	Domain       Link `json:"domain"`
	Destinations Link `json:"destinations"`
}

type routeDestinationsLinks struct {
	Self  Link `json:"self"`
	Route Link `json:"route"`
}

func ForRoute(route repositories.RouteRecord, baseURL url.URL, includes ...include.Resource) RouteResponse {
	destinations := make([]routeDestination, 0, len(route.Destinations))
	for _, destinationRecord := range route.Destinations {
		destinations = append(destinations, forDestination(destinationRecord))
	}
	return RouteResponse{
		GUID:          route.GUID,
		Protocol:      route.Protocol,
		Host:          route.Host,
		Path:          route.Path,
		URL:           routeURL(route),
		CreatedAt:     tools.ZeroIfNil(formatTimestamp(&route.CreatedAt)),
		UpdatedAt:     tools.ZeroIfNil(formatTimestamp(route.UpdatedAt)),
		Relationships: ForRelationships(route.Relationships()),
		Destinations:  destinations,
		Metadata: Metadata{
			Labels:      emptyMapIfNil(route.Labels),
			Annotations: emptyMapIfNil(route.Annotations),
		},
		Links: routeLinks{
			Self: Link{
				HRef: buildURL(baseURL).appendPath(routesBase, route.GUID).build(),
			},
			Space: Link{
				HRef: buildURL(baseURL).appendPath(spacesBase, route.SpaceGUID).build(),
			},
			Domain: Link{
				HRef: buildURL(baseURL).appendPath(domainsBase, route.Domain.GUID).build(),
			},
			Destinations: Link{
				HRef: buildURL(baseURL).appendPath(routesBase, route.GUID, "destinations").build(),
			},
		},
	}
}

func forDestination(destination repositories.DestinationRecord) routeDestination {
	return routeDestination{
		GUID: destination.GUID,
		App: routeDestinationApp{
			AppGUID: destination.AppGUID,
			Process: routeDestinationAppProcess{
				Type: destination.ProcessType,
			},
		},
		Weight:   nil,
		Port:     destination.Port,
		Protocol: destination.Protocol,
	}
}

func ForRouteDestinations(route repositories.RouteRecord, baseURL url.URL) RouteDestinationsResponse {
	destinations := make([]routeDestination, 0, len(route.Destinations))
	for _, destinationRecord := range route.Destinations {
		destinations = append(destinations, forDestination(destinationRecord))
	}
	return RouteDestinationsResponse{
		Destinations: destinations,
		Links: routeDestinationsLinks{
			Self: Link{
				HRef: buildURL(baseURL).appendPath(routesBase, route.GUID, "destinations").build(),
			},
			Route: Link{
				HRef: buildURL(baseURL).appendPath(routesBase, route.GUID).build(),
			},
		},
	}
}

func routeURL(route repositories.RouteRecord) string {
	if route.Host != "" {
		return fmt.Sprintf("%s.%s%s", route.Host, route.Domain.Name, route.Path)
	} else {
		return fmt.Sprintf("%s%s", route.Domain.Name, route.Path)
	}
}
