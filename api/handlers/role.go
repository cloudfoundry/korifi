package handlers

import (
	"context"
	"net/http"
	"net/url"
	"sort"

	"code.cloudfoundry.org/korifi/api/authorization"
	apierrors "code.cloudfoundry.org/korifi/api/errors"
	"code.cloudfoundry.org/korifi/api/payloads"
	"code.cloudfoundry.org/korifi/api/presenter"
	"code.cloudfoundry.org/korifi/api/repositories"
	"code.cloudfoundry.org/korifi/api/routing"
	"github.com/go-logr/logr"
	"github.com/google/uuid"
)

const (
	RolesPath = "/v3/roles"
	RolePath  = RolesPath + "/{guid}"
)

//counterfeiter:generate -o fake -fake-name CFRoleRepository . CFRoleRepository

type CFRoleRepository interface {
	CreateRole(context.Context, authorization.Info, repositories.CreateRoleMessage) (repositories.RoleRecord, error)
	ListRoles(context.Context, authorization.Info) ([]repositories.RoleRecord, error)
	GetRole(context.Context, authorization.Info, string) (repositories.RoleRecord, error)
	DeleteRole(context.Context, authorization.Info, repositories.DeleteRoleMessage) error
}

type Role struct {
	apiBaseURL       url.URL
	roleRepo         CFRoleRepository
	requestValidator RequestValidator
}

func NewRole(apiBaseURL url.URL, roleRepo CFRoleRepository, requestValidator RequestValidator) *Role {
	return &Role{
		apiBaseURL:       apiBaseURL,
		roleRepo:         roleRepo,
		requestValidator: requestValidator,
	}
}

func (h *Role) create(r *http.Request) (*routing.Response, error) {
	authInfo, _ := authorization.InfoFromContext(r.Context())
	logger := logr.FromContextOrDiscard(r.Context()).WithName("handlers.role.create")

	var payload payloads.RoleCreate
	if err := h.requestValidator.DecodeAndValidateJSONPayload(r, &payload); err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "failed to decode payload")
	}

	role := payload.ToMessage()
	role.GUID = uuid.NewString()

	record, err := h.roleRepo.CreateRole(r.Context(), authInfo, role)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "Failed to create role", "Role Type", role.Type, "Space", role.Space, "User", role.User)
	}

	return routing.NewResponse(http.StatusCreated).WithBody(presenter.ForRole(record, h.apiBaseURL)), nil
}

func (h *Role) list(r *http.Request) (*routing.Response, error) {
	authInfo, _ := authorization.InfoFromContext(r.Context())
	logger := logr.FromContextOrDiscard(r.Context()).WithName("handlers.role.list")

	roleListFilter := new(payloads.RoleList)
	err := h.requestValidator.DecodeAndValidateURLValues(r, roleListFilter)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "Unable to decode request query parameters")
	}

	roles, err := h.roleRepo.ListRoles(r.Context(), authInfo)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "failed to list roles")
	}

	filteredRoles := filterRoles(roleListFilter, roles)
	h.sortList(filteredRoles, roleListFilter.OrderBy)

	return routing.NewResponse(http.StatusOK).WithBody(presenter.ForList(presenter.ForRole, filteredRoles, h.apiBaseURL, *r.URL)), nil
}

func filterRoles(roleListFilter *payloads.RoleList, roles []repositories.RoleRecord) []repositories.RoleRecord {
	var filteredRoles []repositories.RoleRecord
	for _, role := range roles {
		if match(roleListFilter.GUIDs, role.GUID) &&
			match(roleListFilter.Types, role.Type) &&
			match(roleListFilter.SpaceGUIDs, role.Space) &&
			match(roleListFilter.OrgGUIDs, role.Org) &&
			match(roleListFilter.UserGUIDs, role.User) {
			filteredRoles = append(filteredRoles, role)
		}
	}
	return filteredRoles
}

func match(allowedValues map[string]bool, val string) bool {
	return len(allowedValues) == 0 || allowedValues[val]
}

func (h *Role) sortList(roles []repositories.RoleRecord, order string) {
	switch order {
	case "":
	case "created_at":
		sort.Slice(roles, func(i, j int) bool { return roles[i].CreatedAt < roles[j].CreatedAt })
	case "-created_at":
		sort.Slice(roles, func(i, j int) bool { return roles[i].CreatedAt > roles[j].CreatedAt })
	case "updated_at":
		sort.Slice(roles, func(i, j int) bool { return roles[i].UpdatedAt < roles[j].UpdatedAt })
	case "-updated_at":
		sort.Slice(roles, func(i, j int) bool { return roles[i].UpdatedAt > roles[j].UpdatedAt })
	}
}

func (h *Role) delete(r *http.Request) (*routing.Response, error) {
	authInfo, _ := authorization.InfoFromContext(r.Context())
	logger := logr.FromContextOrDiscard(r.Context()).WithName("handlers.role.delete")
	roleGUID := routing.URLParam(r, "guid")

	role, err := h.roleRepo.GetRole(r.Context(), authInfo, roleGUID)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, apierrors.ForbiddenAsNotFound(err), "Failed to fetch role from Kubernetes", "RoleGUID", roleGUID)
	}

	err = h.roleRepo.DeleteRole(r.Context(), authInfo, repositories.DeleteRoleMessage{
		GUID:  roleGUID,
		Space: role.Space,
		Org:   role.Org,
	})
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "Failed to delete role", "RoleGUID", roleGUID)
	}

	return routing.NewResponse(http.StatusAccepted).WithHeader("Location", presenter.JobURLForRedirects(roleGUID, presenter.RoleDeleteOperation, h.apiBaseURL)), nil
}

func (h *Role) UnauthenticatedRoutes() []routing.Route {
	return nil
}

func (h *Role) AuthenticatedRoutes() []routing.Route {
	return []routing.Route{
		{Method: "POST", Pattern: RolesPath, Handler: h.create},
		{Method: "GET", Pattern: RolesPath, Handler: h.list},
		{Method: "DELETE", Pattern: RolePath, Handler: h.delete},
	}
}
