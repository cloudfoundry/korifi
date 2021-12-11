package apis

import (
	"net/http"
	"net/url"
	"strings"

	"code.cloudfoundry.org/cf-k8s-controllers/api/presenter"

	"github.com/go-logr/logr"
	"github.com/gorilla/mux"
)

const (
	JobGetEndpoint  = "/v3/jobs/{guid}"
	syncSpacePrefix = "sync-space.apply_manifest-"
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

	spaceGUID := getSpaceGUID(jobGUID)
	if spaceGUID == "" {
		h.logger.Info("Invalid Job GUID")
		writeNotFoundErrorResponse(w, "Job")
		return
	}

	err := writeJsonResponse(w, presenter.ForJob(jobGUID, spaceGUID, h.serverURL), http.StatusOK)
	if err != nil {
		h.logger.Error(err, "Failed to render response", "Job GUID", jobGUID)
		writeUnknownErrorResponse(w)
	}
}

func getSpaceGUID(jobGUID string) string {
	if strings.HasPrefix(jobGUID, syncSpacePrefix) {
		spaceGUID := strings.Replace(jobGUID, syncSpacePrefix, "", 1)
		return spaceGUID
	}
	return ""
}

func (h *JobHandler) RegisterRoutes(router *mux.Router) {
	router.Path(JobGetEndpoint).Methods("GET").HandlerFunc(h.jobGetHandler)
}
