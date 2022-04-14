package apis

import (
	"fmt"
	"net/http"
	"net/url"
	"regexp"

	"code.cloudfoundry.org/korifi/api/apierrors"
	"code.cloudfoundry.org/korifi/api/authorization"
	"code.cloudfoundry.org/korifi/api/presenter"

	"github.com/go-logr/logr"
	"github.com/gorilla/mux"
)

const (
	JobPath           = "/v3/jobs/{guid}"
	syncSpacePrefix   = "space.apply_manifest"
	appDeletePrefix   = "app.delete"
	orgDeletePrefix   = "org.delete"
	routeDeletePrefix = "route.delete"
	spaceDeletePrefix = "space.delete"
)

const JobResourceType = "Job"

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

func (h *JobHandler) jobGetHandler(_ authorization.Info, r *http.Request) (*HandlerResponse, error) {
	vars := mux.Vars(r)
	jobGUID := vars["guid"]

	jobType, resourceGUID, match := parseJobGUID(jobGUID)

	if !match {
		h.logger.Info("Invalid Job GUID")
		return nil, apierrors.NewNotFoundError(fmt.Errorf("invalid job guid: %s", jobGUID), JobResourceType)
	}

	var jobResponse presenter.JobResponse

	switch jobType {
	case syncSpacePrefix:
		jobResponse = presenter.ForManifestApplyJob(jobGUID, resourceGUID, h.serverURL)
	case appDeletePrefix, orgDeletePrefix, spaceDeletePrefix, routeDeletePrefix:
		jobResponse = presenter.ForDeleteJob(jobGUID, jobType, h.serverURL)
	default:
		h.logger.Info("Invalid Job type: %s", jobType)
		return nil, apierrors.NewNotFoundError(fmt.Errorf("invalid job type: %s", jobType), JobResourceType)
	}

	return NewHandlerResponse(http.StatusOK).WithBody(jobResponse), nil
}

func (h *JobHandler) RegisterRoutes(router *mux.Router) {
	w := NewAuthAwareHandlerFuncWrapper(h.logger)
	router.Path(JobPath).Methods("GET").HandlerFunc(w.Wrap(h.jobGetHandler))
}

func parseJobGUID(jobGUID string) (string, string, bool) {
	// Match job.type-GUID and capture the job type and GUID for later use
	jobRegexp := regexp.MustCompile("([a-z_-]+[.][a-z_]+)-(?:cf-[a-z]+-)?([0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})")
	matches := jobRegexp.FindStringSubmatch(jobGUID)

	if len(matches) != 3 {
		return "", "", false
	} else {
		return matches[1], matches[2], true
	}
}
