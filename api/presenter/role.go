package presenter

import (
	"net/url"
	"time"

	"code.cloudfoundry.org/cf-k8s-controllers/api/repositories"
)

const (
	rolesBase = "/v3/roles"
)

type RoleResponse struct {
	GUID string `json:"guid"`

	CreatedAt     string        `json:"created_at"`
	UpdatedAt     string        `json:"updated_at"`
	Type          string        `json:"type"`
	Relationships Relationships `json:"relationships"`
	Links         RoleLinks     `json:"links"`
}

type RoleLinks struct {
	Self         *Link `json:"self"`
	Space        *Link `json:"space,omitempty"`
	Organization *Link `json:"organization,omitempty"`
}

func ForCreateRole(role repositories.RoleRecord, apiBaseURL url.URL) RoleResponse {
	return toRoleResponse(role, apiBaseURL)
}

func toRoleResponse(role repositories.RoleRecord, apiBaseURL url.URL) RoleResponse {
	resp := RoleResponse{
		GUID:      role.GUID,
		CreatedAt: role.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt: role.CreatedAt.UTC().Format(time.RFC3339),
		Type:      role.Type,
		Relationships: Relationships{
			"user":         Relationship{Data: &RelationshipData{GUID: role.User}},
			"space":        Relationship{Data: nil},
			"organization": Relationship{Data: nil},
		},
		Links: RoleLinks{
			Self: &Link{
				HREF: buildURL(apiBaseURL).appendPath(rolesBase, role.GUID).build(),
			},
		},
	}

	if role.Org != "" {
		resp.Relationships["organization"] = Relationship{Data: &RelationshipData{GUID: role.Org}}
		resp.Links.Organization = &Link{
			HREF: buildURL(apiBaseURL).appendPath(orgsBase, role.Org).build(),
		}
	}

	if role.Space != "" {
		resp.Relationships["space"] = Relationship{Data: &RelationshipData{GUID: role.Space}}
		resp.Links.Space = &Link{
			HREF: buildURL(apiBaseURL).appendPath(spacesBase, role.Space).build(),
		}
	}

	return resp
}
