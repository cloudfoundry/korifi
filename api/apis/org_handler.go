package apis

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	"github.com/go-logr/logr"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
	controllerruntime "sigs.k8s.io/controller-runtime"

	"code.cloudfoundry.org/cf-k8s-controllers/api/authorization"
	"code.cloudfoundry.org/cf-k8s-controllers/api/payloads"
	"code.cloudfoundry.org/cf-k8s-controllers/api/presenter"
	"code.cloudfoundry.org/cf-k8s-controllers/api/repositories"
	"code.cloudfoundry.org/cf-k8s-controllers/controllers/webhooks/workloads"
)

const (
	OrgsEndpoint     = "/v3/organizations"
	OrgDeleteEnpoint = "/v3/organizations/{guid}"
)

//counterfeiter:generate -o fake -fake-name OrgRepository . CFOrgRepository
type CFOrgRepository interface {
	CreateOrg(context.Context, authorization.Info, repositories.CreateOrgMessage) (repositories.OrgRecord, error)
	ListOrgs(context.Context, authorization.Info, repositories.ListOrgsMessage) ([]repositories.OrgRecord, error)
	DeleteOrg(context.Context, authorization.Info, repositories.DeleteOrgMessage) error
}

type OrgHandler struct {
	logger           logr.Logger
	apiBaseURL       url.URL
	orgRepo          CFOrgRepository
	decoderValidator *DecoderValidator
}

func NewOrgHandler(apiBaseURL url.URL, orgRepo CFOrgRepository, decoderValidator *DecoderValidator) *OrgHandler {
	return &OrgHandler{
		logger:           controllerruntime.Log.WithName("Org Handler"),
		apiBaseURL:       apiBaseURL,
		orgRepo:          orgRepo,
		decoderValidator: decoderValidator,
	}
}

func (h *OrgHandler) orgCreateHandler(info authorization.Info, w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	var payload payloads.OrgCreate
	rme := h.decoderValidator.DecodeAndValidateJSONPayload(r, &payload)
	if rme != nil {
		writeRequestMalformedErrorResponse(w, rme)

		return
	}

	org := payload.ToMessage()
	org.GUID = uuid.NewString()

	record, err := h.orgRepo.CreateOrg(r.Context(), info, org)
	if err != nil {
		if workloads.HasErrorCode(err, workloads.DuplicateOrgNameError) {
			errorDetail := fmt.Sprintf("Organization '%s' already exists.", org.Name)
			h.logger.Info(errorDetail)
			writeUnprocessableEntityError(w, errorDetail)
			return
		}

		if authorization.IsInvalidAuth(err) {
			h.logger.Error(err, "unauthorized to create org")
			writeInvalidAuthErrorResponse(w)

			return
		}

		if authorization.IsNotAuthenticated(err) {
			h.logger.Error(err, "unauthorized to create org")
			writeNotAuthenticatedErrorResponse(w)

			return
		}

		if repositories.IsForbiddenError(err) {
			h.logger.Error(err, "not allowed to create orgs")
			writeNotAuthorizedErrorResponse(w)

			return
		}

		h.logger.Error(err, "Failed to create org", "Org Name", payload.Name)
		writeUnknownErrorResponse(w)
		return
	}

	writeResponse(w, http.StatusCreated, presenter.ForCreateOrg(record, h.apiBaseURL))
}

func (h *OrgHandler) orgDeleteHandler(info authorization.Info, w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vars := mux.Vars(r)
	orgGUID := vars["guid"]

	deleteOrgMessage := repositories.DeleteOrgMessage{
		GUID: orgGUID,
	}
	err := h.orgRepo.DeleteOrg(ctx, info, deleteOrgMessage)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")

		switch err.(type) {
		case authorization.InvalidAuthError:
			h.logger.Error(err, "unauthorized to delete org", "OrgGUID", orgGUID)
			writeNotAuthorizedErrorResponse(w)
			return
		case repositories.NotFoundError:
			h.logger.Info("Org not found", "OrgGUID", orgGUID)
			writeNotFoundErrorResponse(w, "Org")
			return
		default:
			h.logger.Error(err, "Failed to delete org", "OrgGUID", orgGUID)
			writeUnknownErrorResponse(w)
			return
		}
	}

	w.Header().Set("Location", fmt.Sprintf("%s/v3/jobs/org.delete-%s", h.apiBaseURL.String(), orgGUID))
	writeResponse(w, http.StatusAccepted, "")
}

func (h *OrgHandler) orgListHandler(info authorization.Info, w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	w.Header().Set("Content-Type", "application/json")

	names := parseCommaSeparatedList(r.URL.Query().Get("names"))

	orgs, err := h.orgRepo.ListOrgs(ctx, info, repositories.ListOrgsMessage{Names: names})
	if err != nil {
		if authorization.IsInvalidAuth(err) {
			h.logger.Error(err, "unauthorized to list orgs")
			writeInvalidAuthErrorResponse(w)

			return
		}

		if authorization.IsNotAuthenticated(err) {
			h.logger.Error(err, "unauthorized to list orgs")
			writeNotAuthenticatedErrorResponse(w)

			return
		}

		h.logger.Error(err, "failed to fetch orgs")
		writeUnknownErrorResponse(w)

		return
	}

	writeResponse(w, http.StatusOK, presenter.ForOrgList(orgs, h.apiBaseURL, *r.URL))
}

func (h *OrgHandler) RegisterRoutes(router *mux.Router) {
	w := NewAuthAwareHandlerFuncWrapper(h.logger)
	router.Path(OrgsEndpoint).Methods("GET").HandlerFunc(w.Wrap(h.orgListHandler))
	router.Path(OrgDeleteEnpoint).Methods("DELETE").HandlerFunc(w.Wrap(h.orgDeleteHandler))
	router.Path(OrgsEndpoint).Methods("POST").HandlerFunc(w.Wrap(h.orgCreateHandler))
}
