package presenter

import (
	"net/url"

	"code.cloudfoundry.org/korifi/api/repositories"
)

const (
	rolesBase = "/v3/roles"
)

type RoleResponse struct {
	GUID          string        `json:"guid"`
	CreatedAt     string        `json:"created_at"`
	UpdatedAt     string        `json:"updated_at"`
	Type          string        `json:"type"`
	Relationships Relationships `json:"relationships"`
	Links         RoleLinks     `json:"links"`
}

type RoleLinks struct {
	Self         *Link `json:"self"`
	User         *Link `json:"user"`
	Space        *Link `json:"space,omitempty"`
	Organization *Link `json:"organization,omitempty"`
}

func ForRole(role repositories.RoleRecord, apiBaseURL url.URL) RoleResponse {
	resp := RoleResponse{
		GUID:      role.GUID,
		CreatedAt: formatTimestamp(&role.CreatedAt),
		UpdatedAt: formatTimestamp(role.UpdatedAt),
		Type:      role.Type,
		Relationships: Relationships{
			"user":         Relationship{Data: &RelationshipData{GUID: role.User}},
			"space":        Relationship{Data: nil},
			"organization": Relationship{Data: nil},
		},
		Links: RoleLinks{
			Self: &Link{
				HRef: buildURL(apiBaseURL).appendPath(rolesBase, role.GUID).build(),
			},
			User: &Link{
				HRef: buildURL(apiBaseURL).appendPath(usersBase, role.User).build(),
			},
		},
	}

	if role.Org != "" {
		resp.Relationships["organization"] = Relationship{Data: &RelationshipData{GUID: role.Org}}
		resp.Links.Organization = &Link{
			HRef: buildURL(apiBaseURL).appendPath(orgsBase, role.Org).build(),
		}
	}

	if role.Space != "" {
		resp.Relationships["space"] = Relationship{Data: &RelationshipData{GUID: role.Space}}
		resp.Links.Space = &Link{
			HRef: buildURL(apiBaseURL).appendPath(spacesBase, role.Space).build(),
		}
	}

	return resp
}
