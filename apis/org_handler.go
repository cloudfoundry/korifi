package apis

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"code.cloudfoundry.org/cf-k8s-api/presenter"
	"code.cloudfoundry.org/cf-k8s-api/repositories"
	"github.com/go-logr/logr"
	"github.com/gorilla/mux"
	controllerruntime "sigs.k8s.io/controller-runtime"
)

const OrgListEndpoint = "/v3/organizations"

//counterfeiter:generate -o fake -fake-name CFOrgRepository . CFOrgRepository

type CFOrgRepository interface {
	FetchOrgs(context.Context, []string) ([]repositories.OrgRecord, error)
}

type OrgHandler struct {
	orgRepo    CFOrgRepository
	logger     logr.Logger
	apiBaseURL string
}

func NewOrgHandler(orgRepo CFOrgRepository, apiBaseURL string) *OrgHandler {
	return &OrgHandler{
		orgRepo:    orgRepo,
		apiBaseURL: apiBaseURL,
		logger:     controllerruntime.Log.WithName("Org Handler"),
	}
}

func (h *OrgHandler) OrgListHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	w.Header().Set("Content-Type", "application/json")

	var names []string
	namesList := r.URL.Query().Get("names")
	if len(namesList) > 0 {
		names = strings.Split(namesList, ",")
	}

	orgs, err := h.orgRepo.FetchOrgs(ctx, names)
	if err != nil {
		writeUnknownErrorResponse(w)

		return
	}

	orgList := presenter.ForOrgList(orgs, h.apiBaseURL)
	json.NewEncoder(w).Encode(orgList)
}

func (h *OrgHandler) RegisterRoutes(router *mux.Router) {
	router.Path(OrgListEndpoint).Methods("GET").HandlerFunc(h.OrgListHandler)
}
