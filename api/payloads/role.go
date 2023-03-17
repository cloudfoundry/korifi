package payloads

import (
	"net/url"
	"strings"

	"code.cloudfoundry.org/korifi/api/repositories"
	rbacv1 "k8s.io/api/rbac/v1"
)

type (
	RoleCreate struct {
		Type          string            `json:"type" validate:"required"`
		Relationships RoleRelationships `json:"relationships" validate:"required"`
	}

	UserRelationship struct {
		Data UserRelationshipData `json:"data" validate:"required"`
	}

	UserRelationshipData struct {
		Username string `json:"username" validate:"required_without=GUID"`
		GUID     string `json:"guid" validate:"required_without=Username"`
		Origin   string `json:"origin"`
	}

	RoleRelationships struct {
		User                     *UserRelationship `json:"user" validate:"required_without=KubernetesServiceAccount"`
		KubernetesServiceAccount *Relationship     `json:"kubernetesServiceAccount" validate:"required_without=User"`
		Space                    *Relationship     `json:"space"`
		Organization             *Relationship     `json:"organization"`
	}

	RoleListFilter struct {
		GUIDs      map[string]bool
		Types      map[string]bool
		SpaceGUIDs map[string]bool
		OrgGUIDs   map[string]bool
		UserGUIDs  map[string]bool
	}
)

func (p RoleCreate) ToMessage() repositories.CreateRoleMessage {
	record := repositories.CreateRoleMessage{
		Type: p.Type,
	}

	if p.Relationships.Space != nil {
		record.Space = p.Relationships.Space.Data.GUID
	}

	if p.Relationships.Organization != nil {
		record.Org = p.Relationships.Organization.Data.GUID
	}

	if p.Relationships.User != nil {
		record.Kind = rbacv1.UserKind
		record.User = p.Relationships.User.Data.Username

		// For UAA Authenticated users, prefix the Origin as our Cluster uses the Orgin:User for
		// Authentication verification (via OIDC prefixs)
		// --kube-apiserver-arg oidc-username-prefix="<origin>:"
		// --kube-apiserver-arg oidc-groups-prefix="<origin>:"
		if p.Relationships.User.Data.Origin != "" {
			record.User = p.Relationships.User.Data.Origin + ":" + record.User
		}

		if p.Relationships.User.Data.GUID != "" {
			record.User = p.Relationships.User.Data.GUID
		}
	} else {
		record.Kind = rbacv1.ServiceAccountKind
		record.User = p.Relationships.KubernetesServiceAccount.Data.GUID
	}

	return record
}

func (r *RoleListFilter) SupportedKeys() []string {
	return []string{"guids", "types", "space_guids", "organization_guids", "user_guids", "order_by", "include", "page"}
}

func (r *RoleListFilter) DecodeFromURLValues(values url.Values) error {
	r.GUIDs = commaSepToSet(values.Get("guids"))
	r.Types = commaSepToSet(values.Get("types"))
	r.SpaceGUIDs = commaSepToSet(values.Get("space_guids"))
	r.OrgGUIDs = commaSepToSet(values.Get("organization_guids"))
	r.UserGUIDs = commaSepToSet(values.Get("user_guids"))
	return nil
}

func commaSepToSet(in string) map[string]bool {
	out := map[string]bool{}
	if in == "" {
		return out
	}

	for _, s := range strings.Split(in, ",") {
		out[s] = true
	}

	return out
}
