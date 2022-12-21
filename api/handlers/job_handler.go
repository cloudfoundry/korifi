package handlers

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"regexp"

	"code.cloudfoundry.org/korifi/api/apierrors"
	"code.cloudfoundry.org/korifi/api/authorization"
	"code.cloudfoundry.org/korifi/api/presenter"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/go-chi/chi"
	"github.com/go-logr/logr"
)

const (
	JobPath            = "/v3/jobs/{guid}"
	syncSpacePrefix    = "space.apply_manifest"
	appDeletePrefix    = "app.delete"
	orgDeletePrefix    = "org.delete"
	routeDeletePrefix  = "route.delete"
	spaceDeletePrefix  = "space.delete"
	domainDeletePrefix = "domain.delete"
)

const JobResourceType = "Job"

type JobHandler struct {
	handlerWrapper *AuthAwareHandlerFuncWrapper
	serverURL      url.URL
}

func NewJobHandler(serverURL url.URL) *JobHandler {
	return &JobHandler{
		handlerWrapper: NewAuthAwareHandlerFuncWrapper(ctrl.Log.WithName("JobHandler")),
		serverURL:      serverURL,
	}
}

func (h *JobHandler) jobGetHandler(ctx context.Context, logger logr.Logger, authInfo authorization.Info, r *http.Request) (*HandlerResponse, error) {
	jobGUID := chi.URLParam(r, "guid")

	jobType, resourceGUID, match := parseJobGUID(jobGUID)

	if !match {
		return nil, apierrors.LogAndReturn(
			logger,
			apierrors.NewNotFoundError(fmt.Errorf("invalid job guid: %s", jobGUID), JobResourceType),
			"Invalid Job GUID",
		)
	}

	var jobResponse presenter.JobResponse

	switch jobType {
	case syncSpacePrefix:
		jobResponse = presenter.ForManifestApplyJob(jobGUID, resourceGUID, h.serverURL)
	case appDeletePrefix, orgDeletePrefix, spaceDeletePrefix, routeDeletePrefix, domainDeletePrefix:
		jobResponse = presenter.ForDeleteJob(jobGUID, jobType, h.serverURL)
	default:
		return nil, apierrors.LogAndReturn(
			logger,
			apierrors.NewNotFoundError(fmt.Errorf("invalid job type: %s", jobType), JobResourceType),
			fmt.Sprintf("Invalid Job type: %s", jobType),
		)
	}

	return NewHandlerResponse(http.StatusOK).WithBody(jobResponse), nil
}

func (h *JobHandler) RegisterRoutes(router *chi.Mux) {
	router.Get(JobPath, h.handlerWrapper.Wrap(h.jobGetHandler))
}

func parseJobGUID(jobGUID string) (string, string, bool) {
	// Parse the job identifier and capture the job operation and resource name for later use
	jobOperationPattern := `([a-z_\-]+[\.][a-z_]+)`   // (e.g. app.delete, space.apply_manifest, etc.)
	resourceIdentifierPattern := `([A-Za-z0-9\-\.]+)` // (e.g. cf-space-a4cd478b-0b02-452f-8498-ce87ec5c6649, CUSTOM_ORG_ID, etc.)
	jobRegexp := regexp.MustCompile(jobOperationPattern + presenter.JobGUIDDelimiter + resourceIdentifierPattern)
	matches := jobRegexp.FindStringSubmatch(jobGUID)

	if len(matches) != 3 {
		return "", "", false
	} else {
		return matches[1], matches[2], true
	}
}
