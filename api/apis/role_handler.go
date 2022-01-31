package apis

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"

	"code.cloudfoundry.org/cf-k8s-controllers/api/authorization"
	"code.cloudfoundry.org/cf-k8s-controllers/api/payloads"
	"code.cloudfoundry.org/cf-k8s-controllers/api/presenter"
	"code.cloudfoundry.org/cf-k8s-controllers/api/repositories"
	"github.com/go-logr/logr"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
	controllerruntime "sigs.k8s.io/controller-runtime"
)

const (
	RolesEndpoint = "/v3/roles"
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

type RoleHandler struct {
	logger           logr.Logger
	apiBaseURL       url.URL
	roleRepo         CFRoleRepository
	decoderValidator *DecoderValidator
}

func NewRoleHandler(apiBaseURL url.URL, roleRepo CFRoleRepository, decoderValidator *DecoderValidator) *RoleHandler {
	return &RoleHandler{
		logger:           controllerruntime.Log.WithName("Role Handler"),
		apiBaseURL:       apiBaseURL,
		roleRepo:         roleRepo,
		decoderValidator: decoderValidator,
	}
}

func (h *RoleHandler) roleCreateHandler(authInfo authorization.Info, w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	var payload payloads.RoleCreate
	rme := h.decoderValidator.DecodeAndValidateJSONPayload(r, &payload)
	if rme != nil {
		h.logger.Error(rme, "Failed to parse body")
		writeRequestMalformedErrorResponse(w, rme)

		return
	}

	role := payload.ToMessage()
	role.GUID = uuid.NewString()

	record, err := h.roleRepo.CreateRole(r.Context(), authInfo, role)
	if err != nil {
		if errors.As(err, &repositories.ForbiddenError{}) {
			h.logger.Info("create-role: not authorized", "error", err)
			writeNotAuthorizedErrorResponse(w)
			return
		}
		if errors.Is(err, repositories.ErrorDuplicateRoleBinding) {
			errorDetail := fmt.Sprintf("User '%s' already has '%s' role", role.User, role.Type)
			h.logger.Info(errorDetail)
			writeUnprocessableEntityError(w, errorDetail)
			return
		}
		if errors.Is(err, repositories.ErrorMissingRoleBindingInParentOrg) {
			h.logger.Info("no rolebinding in parent org", "space", role.Space, "user", role.User)
			errorDetail := "Users cannot be assigned roles in a space if they do not have a role in that space's organization."
			writeUnprocessableEntityError(w, errorDetail)
			return
		}
		h.logger.Error(err, "Failed to create role", "Role Type", role.Type, "Space", role.Space, "User", role.User)
		writeUnknownErrorResponse(w)
		return
	}

	err = writeJsonResponse(w, presenter.ForCreateRole(record, h.apiBaseURL), http.StatusCreated)
	if err != nil {
		h.logger.Error(err, "Failed to render response", "User/Role", role.User+"/"+role.Type)
		writeUnknownErrorResponse(w)
	}
}

func (h *RoleHandler) RegisterRoutes(router *mux.Router) {
	w := NewAuthAwareHandlerFuncWrapper(h.logger)
	router.Path(RolesEndpoint).Methods("POST").HandlerFunc(w.Wrap(h.roleCreateHandler))
}
