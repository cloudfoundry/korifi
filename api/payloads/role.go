package payloads

import (
	"context"
	"net/url"
	"strings"

	"code.cloudfoundry.org/korifi/api/authorization"
	"code.cloudfoundry.org/korifi/api/payloads/validation"
	jellidation "github.com/jellydator/validation"

	"code.cloudfoundry.org/korifi/api/repositories"
	rbacv1 "k8s.io/api/rbac/v1"
)

const (
	RoleAdmin                      = "admin"
	RoleAdminReadOnly              = "admin_read_only"
	RoleGlobalAuditor              = "global_auditor"
	RoleOrganizationAuditor        = "organization_auditor"
	RoleOrganizationBillingManager = "organization_billing_manager"
	RoleOrganizationManager        = "organization_manager"
	RoleOrganizationUser           = "organization_user"
	RoleSpaceAuditor               = "space_auditor"
	RoleSpaceDeveloper             = "space_developer"
	RoleSpaceManager               = "space_manager"
	RoleSpaceSupporter             = "space_supporter"
)

type RoleCreate struct {
	Type          string            `json:"type"`
	Relationships RoleRelationships `json:"relationships"`
}

type ctxType string

const typeKey ctxType = "type"

func (p RoleCreate) Validate() error {
	ctx := context.WithValue(context.Background(), typeKey, p.Type)

	return jellidation.ValidateStructWithContext(ctx, &p,
		jellidation.Field(&p.Type,
			jellidation.Required,
			validation.OneOf(RoleSpaceManager, RoleSpaceAuditor, RoleSpaceDeveloper, RoleSpaceSupporter,
				RoleOrganizationUser, RoleOrganizationAuditor, RoleOrganizationManager, RoleOrganizationBillingManager),
		),
		jellidation.Field(&p.Relationships, validation.StrictlyRequired),
	)
}

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

type RoleRelationships struct {
	User         UserRelationship `json:"user"`
	Space        *Relationship    `json:"space"`
	Organization *Relationship    `json:"organization"`
}

func (r RoleRelationships) ValidateWithContext(ctx context.Context) error {
	roleType := ctx.Value(typeKey)

	return jellidation.ValidateStruct(&r,
		jellidation.Field(&r.User, validation.StrictlyRequired),

		jellidation.Field(&r.Space,
			jellidation.When(r.Organization != nil,
				jellidation.Nil.Error("cannot pass both 'organization' and 'space' in a create role request"))),

		jellidation.Field(&r.Space,
			jellidation.When(
				roleType == RoleSpaceAuditor || roleType == RoleSpaceDeveloper ||
					roleType == RoleSpaceManager || roleType == RoleSpaceSupporter,
				jellidation.NotNil,
			)),

		jellidation.Field(&r.Organization,
			jellidation.When(
				roleType == RoleOrganizationAuditor || roleType == RoleOrganizationBillingManager ||
					roleType == RoleOrganizationManager || roleType == RoleOrganizationUser,
				jellidation.NotNil,
			)),
	)
}

type UserRelationship struct {
	Data UserRelationshipData `json:"data"`
}

type UserRelationshipData struct {
	Username string `json:"username"`
	GUID     string `json:"guid"`
}

type RoleList struct {
	GUIDs      map[string]bool
	Types      map[string]bool
	SpaceGUIDs map[string]bool
	OrgGUIDs   map[string]bool
	UserGUIDs  map[string]bool
	OrderBy    string
}

func (r RoleList) SupportedKeys() []string {
	return []string{"guids", "types", "space_guids", "organization_guids", "user_guids", "order_by", "include", "per_page", "page"}
}

func (r *RoleList) DecodeFromURLValues(values url.Values) error {
	r.GUIDs = commaSepToSet(values.Get("guids"))
	r.Types = commaSepToSet(values.Get("types"))
	r.SpaceGUIDs = commaSepToSet(values.Get("space_guids"))
	r.OrgGUIDs = commaSepToSet(values.Get("organization_guids"))
	r.UserGUIDs = commaSepToSet(values.Get("user_guids"))
	r.OrderBy = values.Get("order_by")
	return nil
}

func (r RoleList) Validate() error {
	return jellidation.ValidateStruct(&r,
		jellidation.Field(&r.OrderBy, validation.OneOf("created_at", "updated_at", "-created_at", "-updated_at")),
	)
}

func commaSepToSet(in string) map[string]bool {
	if in == "" {
		return nil
	}

	out := map[string]bool{}
	for _, s := range strings.Split(in, ",") {
		out[s] = true
	}

	return out
}
