package presenter

import (
	"fmt"
	"net/url"

	"code.cloudfoundry.org/cf-k8s-api/repositories"
)

const (
	routesBase  = "/v3/routes"
	domainsBase = "/v3/domains"
)

type RouteResponse struct {
	GUID         string             `json:"guid"`
	Protocol     string             `json:"protocol"`
	Port         *int               `json:"port"`
	Host         string             `json:"host"`
	Path         string             `json:"path"`
	URL          string             `json:"url"`
	Destinations []routeDestination `json:"destinations"`

	CreatedAt     string        `json:"created_at"`
	UpdatedAt     string        `json:"updated_at"`
	Relationships Relationships `json:"relationships"`
	Metadata      Metadata      `json:"metadata"`
	Links         routeLinks    `json:"links"`
}

type routeDestination struct {
	App routeDestinationApp `json:"app"`
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

func ForRoute(route repositories.RouteRecord, baseURL url.URL) RouteResponse {
	return RouteResponse{
		GUID:      route.GUID,
		Protocol:  route.Protocol,
		Host:      route.Host,
		Path:      route.Path,
		URL:       routeURL(route),
		CreatedAt: route.CreatedAt,
		UpdatedAt: route.UpdatedAt,
		Relationships: Relationships{
			"space": Relationship{
				Data: RelationshipData{
					GUID: route.SpaceGUID,
				},
			},
			"domain": Relationship{
				Data: RelationshipData{
					GUID: route.DomainRef.GUID,
				},
			},
		},
		Metadata: Metadata{
			Labels:      map[string]string{},
			Annotations: map[string]string{},
		},
		Links: routeLinks{
			Self: Link{
				HREF: buildURL(baseURL).appendPath(routesBase, route.GUID).build(),
			},
			Space: Link{
				HREF: buildURL(baseURL).appendPath(spacesBase, route.SpaceGUID).build(),
			},
			Domain: Link{
				HREF: buildURL(baseURL).appendPath(domainsBase, route.DomainRef.GUID).build(),
			},
			Destinations: Link{
				HREF: buildURL(baseURL).appendPath(routesBase, route.GUID, "destinations").build(),
			},
		},
	}
}

func routeURL(route repositories.RouteRecord) string {
	if route.Host != "" {
		return fmt.Sprintf("%s.%s%s", route.Host, route.DomainRef.Name, route.Path)
	} else {
		return fmt.Sprintf("%s%s", route.DomainRef.Name, route.Path)
	}
}
