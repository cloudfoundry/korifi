package apis

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"code.cloudfoundry.org/cf-k8s-controllers/api/payloads"
	"code.cloudfoundry.org/cf-k8s-controllers/api/presenter"
	"code.cloudfoundry.org/cf-k8s-controllers/api/repositories"
	"code.cloudfoundry.org/cf-k8s-controllers/api/repositories/authorization"
	"code.cloudfoundry.org/cf-k8s-controllers/controllers/webhooks/workloads"
	"github.com/go-logr/logr"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
	controllerruntime "sigs.k8s.io/controller-runtime"
)

const (
	OrgsEndpoint = "/v3/organizations"
)

//counterfeiter:generate -o fake -fake-name OrgRepositoryProvider . OrgRepositoryProvider

type OrgRepositoryProvider interface {
	OrgRepoForRequest(request *http.Request) (repositories.CFOrgRepository, error)
}

type OrgHandler struct {
	logger          logr.Logger
	apiBaseURL      url.URL
	orgRepoProvider OrgRepositoryProvider
}

func NewOrgHandler(apiBaseURL url.URL, orgRepoProvider OrgRepositoryProvider) *OrgHandler {
	return &OrgHandler{
		logger:          controllerruntime.Log.WithName("Org Handler"),
		apiBaseURL:      apiBaseURL,
		orgRepoProvider: orgRepoProvider,
	}
}

func (h *OrgHandler) orgCreateHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	var payload payloads.OrgCreate
	rme := decodeAndValidateJSONPayload(r, &payload)
	if rme != nil {
		writeRequestMalformedErrorResponse(w, rme)

		return
	}

	org := payload.ToRecord()
	org.GUID = uuid.NewString()

	orgRepo, err := h.orgRepoProvider.OrgRepoForRequest(r)
	if err != nil {
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

		h.logger.Error(err, "failed to create org repo for the authorization header")
		writeUnknownErrorResponse(w)

		return
	}

	record, err := orgRepo.CreateOrg(r.Context(), org)
	if err != nil {
		if workloads.HasErrorCode(err, workloads.DuplicateOrgNameError) {
			errorDetail := fmt.Sprintf("Organization '%s' already exists.", org.Name)
			h.logger.Info(errorDetail)
			writeUnprocessableEntityError(w, errorDetail)
			return
		}
		h.logger.Error(err, "Failed to create org", "Org Name", payload.Name)
		writeUnknownErrorResponse(w)
		return
	}

	orgResponse := presenter.ForCreateOrg(record, h.apiBaseURL)
	writeResponse(w, http.StatusCreated, orgResponse)
}

func (h *OrgHandler) orgListHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	w.Header().Set("Content-Type", "application/json")

	var names []string
	namesList := r.URL.Query().Get("names")
	if len(namesList) > 0 {
		names = strings.Split(namesList, ",")
	}

	orgRepo, err := h.orgRepoProvider.OrgRepoForRequest(r)
	if err != nil {
		if authorization.IsNotAuthenticated(err) {
			h.logger.Error(err, "unauthorized to list orgs")
			writeNotAuthenticatedErrorResponse(w)

			return
		}

		if authorization.IsInvalidAuth(err) {
			h.logger.Error(err, "unauthorized to list orgs")
			writeInvalidAuthErrorResponse(w)

			return
		}

		h.logger.Error(err, "failed to create org repo for the authorization header")
		writeUnknownErrorResponse(w)

		return
	}

	orgs, err := orgRepo.FetchOrgs(ctx, names)
	if err != nil {
		h.logger.Error(err, "failed to fetch orgs")
		writeUnknownErrorResponse(w)

		return
	}

	orgList := presenter.ForOrgList(orgs, h.apiBaseURL)
	writeResponse(w, http.StatusOK, orgList)
}

func (h *OrgHandler) RegisterRoutes(router *mux.Router) {
	router.Path(OrgsEndpoint).Methods("GET").HandlerFunc(h.orgListHandler)
	router.Path(OrgsEndpoint).Methods("POST").HandlerFunc(h.orgCreateHandler)
}
