package presenter

import (
	"net/url"

	"code.cloudfoundry.org/cf-k8s-controllers/api/repositories"
)

const (
	domainsBase = "/v3/domains"
)

type DomainResponse struct {
	Name               string   `json:"name"`
	GUID               string   `json:"guid"`
	Internal           bool     `json:"internal"`
	RouterGroup        *string  `json:"router_group"`
	SupportedProtocols []string `json:"supported_protocols"`

	CreatedAt     string              `json:"created_at"`
	UpdatedAt     string              `json:"updated_at"`
	Metadata      Metadata            `json:"metadata"`
	Relationships DomainRelationships `json:"relationships"`
	Links         DomainLinks         `json:"links"`
}

type DomainLinks struct {
	Self              Link  `json:"self"`
	RouteReservations Link  `json:"route_reservations"`
	RouterGroup       *Link `json:"router_group"`
}

type DomainRelationships struct {
	Organization        `json:"organization"`
	SharedOrganizations `json:"shared_organizations"`
}

type Organization struct {
	Data *string `json:"data"`
}

type SharedOrganizations struct {
	Data []string `json:"data"`
}

func ForDomain(responseDomain repositories.DomainRecord, baseURL url.URL) DomainResponse {
	return DomainResponse{
		Name:               responseDomain.Name,
		GUID:               responseDomain.GUID,
		Internal:           false,
		RouterGroup:        nil,
		SupportedProtocols: []string{"http"},
		CreatedAt:          responseDomain.CreatedAt,
		UpdatedAt:          responseDomain.UpdatedAt,

		Metadata: Metadata{
			Labels:      map[string]string{},
			Annotations: map[string]string{},
		},
		Relationships: DomainRelationships{
			Organization: Organization{
				Data: nil,
			},
			SharedOrganizations: SharedOrganizations{
				Data: []string{},
			},
		},
		Links: DomainLinks{
			Self: Link{
				HREF: buildURL(baseURL).appendPath(domainsBase, responseDomain.GUID).build(),
			},
			RouteReservations: Link{
				HREF: buildURL(baseURL).appendPath(domainsBase, responseDomain.GUID, "route_reservations").build(),
			},
			RouterGroup: nil,
		},
	}
}

func ForDomainList(domainListRecords []repositories.DomainRecord, baseURL, requestURL url.URL) ListResponse {
	domainResponses := make([]interface{}, 0, len(domainListRecords))
	for _, domain := range domainListRecords {
		domainResponses = append(domainResponses, ForDomain(domain, baseURL))
	}

	return ForList(domainResponses, baseURL, requestURL)
}
