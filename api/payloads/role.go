package payloads

import (
	"net/url"
	"strings"

	"code.cloudfoundry.org/korifi/api/authorization"

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
	}

	RoleRelationships struct {
		User         *UserRelationship `json:"user" validate:"required"`
		Space        *Relationship     `json:"space"`
		Organization *Relationship     `json:"organization"`
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

	record.Kind = rbacv1.UserKind
	record.User = p.Relationships.User.Data.Username
	if p.Relationships.User.Data.GUID != "" {
		record.User = p.Relationships.User.Data.GUID
	}

	if authorization.HasServiceAccountPrefix(record.User) {
		namespace, user := authorization.ServiceAccountNSAndName(record.User)

		record.Kind = rbacv1.ServiceAccountKind
		record.User = user
		record.ServiceAccountNamespace = namespace
	}

	return record
}

func (r *RoleListFilter) SupportedKeys() []string {
	return []string{"guids", "types", "space_guids", "organization_guids", "user_guids", "order_by", "include"}
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
