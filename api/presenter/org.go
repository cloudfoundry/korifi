package presenter

import (
	"net/url"
	"time"

	"code.cloudfoundry.org/korifi/api/repositories"
)

const (
	// TODO: repetition with handler endpoint?
	orgsBase = "/v3/organizations"
)

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

func ForCreateOrg(org repositories.OrgRecord, apiBaseURL url.URL) OrgResponse {
	return toOrgResponse(org, apiBaseURL)
}

func ForOrgList(orgs []repositories.OrgRecord, apiBaseURL, requestURL url.URL) ListResponse {
	orgResponses := make([]interface{}, 0, len(orgs))
	for _, org := range orgs {
		orgResponses = append(orgResponses, toOrgResponse(org, apiBaseURL))
	}

	return ForList(orgResponses, apiBaseURL, requestURL)
}

func toOrgResponse(org repositories.OrgRecord, apiBaseURL url.URL) OrgResponse {
	return OrgResponse{
		Name:      org.Name,
		GUID:      org.GUID,
		CreatedAt: org.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt: org.CreatedAt.UTC().Format(time.RFC3339),
		Suspended: org.Suspended,
		Metadata: Metadata{
			Labels:      orEmptyMap(org.Labels),
			Annotations: orEmptyMap(org.Annotations),
		},
		Relationships: Relationships{},
		Links: OrgLinks{
			Self: &Link{
				HREF: buildURL(apiBaseURL).appendPath(orgsBase, org.GUID).build(),
			},
		},
	}
}

func orEmptyMap(m map[string]string) map[string]string {
	if m == nil {
		return map[string]string{}
	}

	return m
}
