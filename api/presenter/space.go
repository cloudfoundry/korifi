package presenter

import (
	"net/url"
	"slices"

	"code.cloudfoundry.org/korifi/api/repositories"
	"code.cloudfoundry.org/korifi/api/repositories/include"
	"code.cloudfoundry.org/korifi/tools"
	"github.com/BooleanCat/go-functional/v2/it"
	"github.com/BooleanCat/go-functional/v2/it/itx"
)

const (
	// TODO: repetition with handler endpoint?
	OrganizationsLabel = "organizations"
	spacesBase         = "/v3/spaces"
)

type SpaceResponse struct {
	Name          string                       `json:"name"`
	GUID          string                       `json:"guid"`
	CreatedAt     string                       `json:"created_at"`
	UpdatedAt     string                       `json:"updated_at"`
	Links         SpaceLinks                   `json:"links"`
	Metadata      Metadata                     `json:"metadata"`
	Relationships map[string]ToOneRelationship `json:"relationships"`
	Included      map[string][]any             `json:"included,omitempty"`
}

type SpaceLinks struct {
	Self         *Link `json:"self"`
	Organization *Link `json:"organization"`
}

func ForSpaceList(spaces []repositories.SpaceRecord, orgs []repositories.OrgRecord, baseURL, requestURL url.URL) ListResponse[SpaceResponse] {
	includedOrgs := slices.Collect(it.Map(itx.FromSlice(orgs), func(org repositories.OrgRecord) include.Resource {
		return include.Resource{
			Type:     OrganizationsLabel,
			Resource: ForOrg(org, baseURL),
		}
	}))

	return ForList(ForSpace, spaces, baseURL, requestURL, includedOrgs...)
}

func ForSpace(space repositories.SpaceRecord, apiBaseURL url.URL, includes ...include.Resource) SpaceResponse {
	var resources map[string][]any
	if includes != nil {
		incl := includes[0]
		resources = map[string][]any{
			incl.Type: []any{incl.Resource},
		}
	}

	return SpaceResponse{
		Name:      space.Name,
		GUID:      space.GUID,
		CreatedAt: tools.ZeroIfNil(formatTimestamp(&space.CreatedAt)),
		UpdatedAt: tools.ZeroIfNil(formatTimestamp(space.UpdatedAt)),
		Metadata: Metadata{
			Labels:      emptyMapIfNil(space.Labels),
			Annotations: emptyMapIfNil(space.Annotations),
		},
		Relationships: ForRelationships(space.Relationships()),
		Links: SpaceLinks{
			Self: &Link{
				HRef: buildURL(apiBaseURL).appendPath(spacesBase, space.GUID).build(),
			},
			Organization: &Link{
				HRef: buildURL(apiBaseURL).appendPath(orgsBase, space.OrganizationGUID).build(),
			},
		},
		Included: resources,
	}
}
