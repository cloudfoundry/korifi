package presenter

import (
	"net/url"
	"time"

	"code.cloudfoundry.org/korifi/api/repositories"
	"code.cloudfoundry.org/korifi/api/repositories/include"
	"code.cloudfoundry.org/korifi/tools"
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

	CreatedAt     time.Time           `json:"created_at"`
	UpdatedAt     time.Time           `json:"updated_at"`
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

func ForDomain(responseDomain repositories.DomainRecord, baseURL url.URL, includes ...include.Resource) DomainResponse {
	return DomainResponse{
		Name:               responseDomain.Name,
		GUID:               responseDomain.GUID,
		Internal:           false,
		RouterGroup:        nil,
		SupportedProtocols: []string{"http"},
		CreatedAt:          tools.ZeroIfNil(toUTC(&responseDomain.CreatedAt)),
		UpdatedAt:          tools.ZeroIfNil(toUTC(responseDomain.UpdatedAt)),

		Metadata: Metadata{
			Labels:      emptyMapIfNil(responseDomain.Labels),
			Annotations: emptyMapIfNil(responseDomain.Annotations),
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
				HRef: buildURL(baseURL).appendPath(domainsBase, responseDomain.GUID).build(),
			},
			RouteReservations: Link{
				HRef: buildURL(baseURL).appendPath(domainsBase, responseDomain.GUID, "route_reservations").build(),
			},
			RouterGroup: nil,
		},
	}
}
