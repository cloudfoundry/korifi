package apis

import (
	"encoding/json"
	"net/http"
	"net/url"
	"regexp"

	"code.cloudfoundry.org/cf-k8s-controllers/api/presenter"

	"github.com/go-logr/logr"
	"github.com/gorilla/mux"
)

const (
	JobGetEndpoint    = "/v3/jobs/{guid}"
	syncSpacePrefix   = "space.apply_manifest"
	appDeletePrefix   = "app.delete"
	orgDeletePrefix   = "org.delete"
	routeDeletePrefix = "route.delete"
	spaceDeletePrefix = "space.delete"
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

	jobType, resourceGUID, match := parseJobGUID(jobGUID)

	if !match {
		h.logger.Info("Invalid Job GUID")
		writeNotFoundErrorResponse(w, "Job")
		return
	}

	var jobResponse presenter.JobResponse

	switch jobType {
	case syncSpacePrefix:
		jobResponse = presenter.ForManifestApplyJob(jobGUID, resourceGUID, h.serverURL)
	case appDeletePrefix, orgDeletePrefix, spaceDeletePrefix, routeDeletePrefix:
		jobResponse = presenter.ForDeleteJob(jobGUID, jobType, h.serverURL)
	default:
		h.logger.Info("Invalid Job type: %s", jobType)
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

func parseJobGUID(jobGUID string) (string, string, bool) {
	// Match job.type-GUID and capture the job type and GUID for later use
	jobRegexp := regexp.MustCompile("([a-z_-]+[.][a-z_]+)-([0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})")
	matches := jobRegexp.FindStringSubmatch(jobGUID)

	if len(matches) != 3 {
		return "", "", false
	} else {
		return matches[1], matches[2], true
	}
}
