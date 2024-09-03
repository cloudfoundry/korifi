package presenter

import (
	"net/url"

	"code.cloudfoundry.org/korifi/api/repositories"
	"code.cloudfoundry.org/korifi/model"
)

const (
	// TODO: repetition with handler endpoint?
	spacesBase = "/v3/spaces"
)

type SpaceResponse struct {
	Name          string                             `json:"name"`
	GUID          string                             `json:"guid"`
	CreatedAt     string                             `json:"created_at"`
	UpdatedAt     string                             `json:"updated_at"`
	Links         SpaceLinks                         `json:"links"`
	Metadata      Metadata                           `json:"metadata"`
	Relationships map[string]model.ToOneRelationship `json:"relationships"`
}

type SpaceLinks struct {
	Self         *Link `json:"self"`
	Organization *Link `json:"organization"`
}

func ForSpace(space repositories.SpaceRecord, apiBaseURL url.URL) SpaceResponse {
	return SpaceResponse{
		Name:      space.Name,
		GUID:      space.GUID,
		CreatedAt: formatTimestamp(&space.CreatedAt),
		UpdatedAt: formatTimestamp(space.UpdatedAt),
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
	}
}
