package presenter

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
	Destinations  []routeDestination `json:"destinations"`
	Relationships Relationships      `json:"relationships"`
	Metadata      Metadata           `json:"metadata"`
	Links         routeLinks         `json:"links"`
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

func ForRoute(route repositories.RouteRecord, baseURL string) RouteResponse {
	routeResponse := RouteResponse{
		GUID:     route.GUID,
		Protocol: route.Protocol,
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
		Links: routeLinks{
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
