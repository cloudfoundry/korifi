package handlers

import (
	"context"
	"net/http"
	"net/url"

	"code.cloudfoundry.org/korifi/api/authorization"
	apierrors "code.cloudfoundry.org/korifi/api/errors"
	"code.cloudfoundry.org/korifi/api/payloads"
	"code.cloudfoundry.org/korifi/api/presenter"
	"code.cloudfoundry.org/korifi/api/repositories"
	"code.cloudfoundry.org/korifi/api/routing"
	"github.com/go-chi/chi"
	"github.com/go-logr/logr"
	"github.com/google/uuid"
)

const (
	RolesPath = "/v3/roles"
)

type RoleName string

const (
	RoleAdmin                      RoleName = "admin"
	RoleAdminReadOnly              RoleName = "admin_read_only"
	RoleGlobalAuditor              RoleName = "global_auditor"
	RoleOrganizationAuditor        RoleName = "organization_auditor"
	RoleOrganizationBillingManager RoleName = "organization_billing_manager"
	RoleOrganizationManager        RoleName = "organization_manager"
	RoleOrganizationUser           RoleName = "organization_user"
	RoleSpaceAuditor               RoleName = "space_auditor"
	RoleSpaceDeveloper             RoleName = "space_developer"
	RoleSpaceManager               RoleName = "space_manager"
	RoleSpaceSupporter             RoleName = "space_supporter"
)

//counterfeiter:generate -o fake -fake-name CFRoleRepository . CFRoleRepository

type CFRoleRepository interface {
	CreateRole(context.Context, authorization.Info, repositories.CreateRoleMessage) (repositories.RoleRecord, error)
}

type Role struct {
	apiBaseURL       url.URL
	roleRepo         CFRoleRepository
	decoderValidator *DecoderValidator
}

func NewRole(apiBaseURL url.URL, roleRepo CFRoleRepository, decoderValidator *DecoderValidator) *Role {
	return &Role{
		apiBaseURL:       apiBaseURL,
		roleRepo:         roleRepo,
		decoderValidator: decoderValidator,
	}
}

func (h *Role) create(r *http.Request) (*routing.Response, error) {
	authInfo, _ := authorization.InfoFromContext(r.Context())
	logger := logr.FromContextOrDiscard(r.Context()).WithName("handlers.role.create")

	var payload payloads.RoleCreate
	if err := h.decoderValidator.DecodeAndValidateJSONPayload(r, &payload); err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "failed to decode payload")
	}

	role := payload.ToMessage()
	role.GUID = uuid.NewString()

	record, err := h.roleRepo.CreateRole(r.Context(), authInfo, role)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "Failed to create role", "Role Type", role.Type, "Space", role.Space, "User", role.User)
	}

	return routing.NewResponse(http.StatusCreated).WithBody(presenter.ForCreateRole(record, h.apiBaseURL)), nil
}

func (h *Role) RegisterRoutes(router *chi.Mux) {
	router.Method("POST", RolesPath, routing.Handler(h.create))
}
