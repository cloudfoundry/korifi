package presenters

import (
	"fmt"

	"code.cloudfoundry.org/cf-k8s-api/repositories"
)

type RouteResponse struct {
	GUID          string             `json:"guid"`
	Protocol      string             `json:"protocol"`
	Port          *int               `json:"port"`
	Host          string             `json:"host"`
	Path          string             `json:"path"`
	URL           string             `json:"url"`
	Destinations  []RouteDestination `json:"destinations"`
	Relationships Relationships      `json:"relationships"`
	Metadata      Metadata           `json:"metadata"`
	Links         RouteLinks         `json:"links"`
}

type RouteDestination struct {
	AppGUID     string `json:"app.guid"`
	ProcessType string `json:"app.process.type"`
}

type RouteLinks struct {
	Self         Link `json:"self"`
	Space        Link `json:"space"`
	Domain       Link `json:"domain"`
	Destinations Link `json:"destinations"`
}

func NewPresentedRoute(route repositories.RouteRecord, baseURL string) RouteResponse {
	routeResponse := RouteResponse{
		GUID:     route.GUID,
		Protocol: string(route.Protocol),
		Host:     route.Host,
		Path:     route.Path,
		URL:      url(route),
		Relationships: Relationships{
			"space": Relationship{
				GUID: route.SpaceGUID,
			},
			"domain": Relationship{
				GUID: route.DomainRef.GUID,
			},
		},
		Metadata: Metadata{
			Labels:      map[string]string{},
			Annotations: map[string]string{},
		},
		Links: RouteLinks{
			Self: Link{
				HREF: prefixedLinkURL(baseURL, fmt.Sprintf("v3/routes/%s", route.GUID)),
			},
			Space: Link{
				HREF: prefixedLinkURL(baseURL, fmt.Sprintf("v3/spaces/%s", route.SpaceGUID)),
			},
			Domain: Link{
				HREF: prefixedLinkURL(baseURL, fmt.Sprintf("v3/domains/%s", route.DomainRef.GUID)),
			},
			Destinations: Link{
				HREF: prefixedLinkURL(baseURL, fmt.Sprintf("v3/routes/%s/destinations", route.GUID)),
			},
		},
	}

	return routeResponse
}

func url(route repositories.RouteRecord) string {
	if route.Host != "" {
		return fmt.Sprintf("%s.%s%s", route.Host, route.DomainRef.Name, route.Path)
	} else {
		return fmt.Sprintf("%s%s", route.DomainRef.Name, route.Path)
	}
}
