package presenter

import (
	neturl "net/url"
	"path"
	"time"

	"code.cloudfoundry.org/cf-k8s-api/repositories"
)

const (
	// TODO: repetition with handler endpoint?
	orgsBase = "/v3/organizations"
)

type OrgListResponse struct {
	Pagination PaginationData `json:"pagination"`
	Resources  []OrgResponse  `json:"resources"`
}

type OrgResponse struct {
	Name string `json:"name"`
	GUID string `json:"guid"`

	CreatedAt     string        `json:"created_at"`
	UpdatedAt     string        `json:"updated_at"`
	Suspended     bool          `json:"suspended"`
	Relationships Relationships `json:"relationships"`
	Metadata      Metadata      `json:"metadata"`
	Links         OrgLinks      `json:"links"`
}

type OrgLinks struct {
	Self          *Link `json:"self"`
	Domains       *Link `json:"domains,omitempty"`
	DefaultDomain *Link `json:"default_domain,omitempty"`
	Quota         *Link `json:"quota,omitempty"`
}

func ForOrgList(orgs []repositories.OrgRecord, apiBaseURL string) OrgListResponse {
	baseURL, _ := neturl.Parse(apiBaseURL)
	baseURL.Path = orgsBase
	baseURL.RawQuery = "page=1"

	selfLink, _ := neturl.Parse(apiBaseURL)

	orgResponses := []OrgResponse{}
	for _, org := range orgs {
		selfLink.Path = path.Join(orgsBase, org.GUID)
		orgResponses = append(orgResponses, OrgResponse{
			Name:      org.Name,
			GUID:      org.GUID,
			CreatedAt: org.CreatedAt.UTC().Format(time.RFC3339),
			UpdatedAt: org.CreatedAt.UTC().Format(time.RFC3339),
			Metadata: Metadata{
				Labels:      map[string]string{},
				Annotations: map[string]string{},
			},
			Relationships: Relationships{},
			Links: OrgLinks{
				Self: &Link{
					HREF: selfLink.String(),
				},
			},
		})
	}

	return OrgListResponse{
		Pagination: PaginationData{
			TotalResults: len(orgs),
			TotalPages:   1,
			First: PageRef{
				HREF: prefixedLinkURL(apiBaseURL, "v3/organizations?page=1"),
			},
			Last: PageRef{
				HREF: prefixedLinkURL(apiBaseURL, "v3/organizations?page=1"),
			},
		},
		Resources: orgResponses,
	}
}
