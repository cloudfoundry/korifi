package payloads

import (
	"context"
	"net/url"

	"code.cloudfoundry.org/korifi/api/authorization"
	"code.cloudfoundry.org/korifi/api/payloads/parse"
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
	message := repositories.CreateRoleMessage{
		Type: p.Type,
	}

	if p.Relationships.Space != nil {
		message.Space = p.Relationships.Space.Data.GUID
	}

	if p.Relationships.Organization != nil {
		message.Org = p.Relationships.Organization.Data.GUID
	}

	message.Kind = rbacv1.UserKind
	message.User = p.Relationships.User.Data.Username

	// For UAA Authenticated users, prefix the Origin as our Cluster uses the Orgin:User for
	// Authentication verification (via OIDC prefixs)
	// --kube-apiserver-arg oidc-username-prefix="<origin>:"
	// --kube-apiserver-arg oidc-groups-prefix="<origin>:"
	if p.Relationships.User.Data.Origin != "" {
		message.User = p.Relationships.User.Data.Origin + ":" + message.User
	}

	if p.Relationships.User.Data.GUID != "" {
		message.User = p.Relationships.User.Data.GUID
	}

	if authorization.HasServiceAccountPrefix(message.User) {
		namespace, user := authorization.ServiceAccountNSAndName(message.User)

		message.Kind = rbacv1.ServiceAccountKind
		message.User = user
		message.ServiceAccountNamespace = namespace
	}

	return message
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
	Origin   string `json:"origin"`
}

type RoleList struct {
	GUIDs      string
	Types      string
	SpaceGUIDs string
	OrgGUIDs   string
	UserGUIDs  string
	OrderBy    string
	Pagination Pagination
}

func (r RoleList) ToMessage() repositories.ListRolesMessage {
	return repositories.ListRolesMessage{
		GUIDs:      parse.ArrayParam(r.GUIDs),
		Types:      parse.ArrayParam(r.Types),
		SpaceGUIDs: parse.ArrayParam(r.SpaceGUIDs),
		OrgGUIDs:   parse.ArrayParam(r.OrgGUIDs),
		UserGUIDs:  parse.ArrayParam(r.UserGUIDs),
		OrderBy:    r.OrderBy,
		Pagination: r.Pagination.ToMessage(DefaultPageSize),
	}
}

func (r RoleList) SupportedKeys() []string {
	return []string{"guids", "types", "space_guids", "organization_guids", "user_guids", "order_by", "include", "per_page", "page"}
}

func (r *RoleList) DecodeFromURLValues(values url.Values) error {
	r.GUIDs = values.Get("guids")
	r.Types = values.Get("types")
	r.SpaceGUIDs = values.Get("space_guids")
	r.OrgGUIDs = values.Get("organization_guids")
	r.UserGUIDs = values.Get("user_guids")
	r.OrderBy = values.Get("order_by")
	return r.Pagination.DecodeFromURLValues(values)
}

func (r RoleList) Validate() error {
	return jellidation.ValidateStruct(&r,
		jellidation.Field(&r.OrderBy, validation.OneOfOrderBy("created_at", "updated_at")),
		jellidation.Field(&r.Pagination),
	)
}
