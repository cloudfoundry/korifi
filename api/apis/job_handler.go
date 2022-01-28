package apis

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"

	"code.cloudfoundry.org/cf-k8s-controllers/api/presenter"

	"github.com/go-logr/logr"
	"github.com/gorilla/mux"
)

const (
	JobGetEndpoint    = "/v3/jobs/{guid}"
	syncSpacePrefix   = "sync-space.apply_manifest-"
	appDeletePrefix   = "app.delete-"
	orgDeletePrefix   = "org.delete-"
	routeDeletePrefix = "route.delete-"
	spaceDeletePrefix = "space.delete-"
)

type JobHandler struct {
	logger    logr.Logger
	serverURL url.URL
}

func NewJobHandler(logger logr.Logger, serverURL url.URL) *JobHandler {
	return &JobHandler{
		logger:    logger,
		serverURL: serverURL,
	}
}

func (h *JobHandler) jobGetHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	vars := mux.Vars(r)
	jobGUID := vars["guid"]

	var jobResponse presenter.JobResponse
	if strings.HasPrefix(jobGUID, syncSpacePrefix) {
		spaceGUID := strings.Replace(jobGUID, syncSpacePrefix, "", 1)
		jobResponse = presenter.ForManifestApplyJob(jobGUID, spaceGUID, h.serverURL)
	} else if strings.HasPrefix(jobGUID, appDeletePrefix) {
		jobResponse = presenter.ForAppDeleteJob(jobGUID, h.serverURL)
	} else if strings.HasPrefix(jobGUID, orgDeletePrefix) {
		jobResponse = presenter.ForOrgDeleteJob(jobGUID, h.serverURL)
	} else if strings.HasPrefix(jobGUID, spaceDeletePrefix) {
		jobResponse = presenter.ForSpaceDeleteJob(jobGUID, h.serverURL)
	} else if strings.HasPrefix(jobGUID, routeDeletePrefix) {
		jobResponse = presenter.ForRouteDeleteJob(jobGUID, h.serverURL)
	} else {
		h.logger.Info("Invalid Job GUID")
		writeNotFoundErrorResponse(w, "Job")
		return
	}

	responseBody, err := json.Marshal(jobResponse)
	if err != nil {
		h.logger.Error(err, "Failed to render response", "Job GUID", jobGUID)
		writeUnknownErrorResponse(w)
		return
	}

	_, _ = w.Write(responseBody)
}

func (h *JobHandler) RegisterRoutes(router *mux.Router) {
	router.Path(JobGetEndpoint).Methods("GET").HandlerFunc(h.jobGetHandler)
}
