package handlers

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"time"

	"code.cloudfoundry.org/korifi/api/authorization"
	apierrors "code.cloudfoundry.org/korifi/api/errors"
	"code.cloudfoundry.org/korifi/api/presenter"
	"code.cloudfoundry.org/korifi/api/repositories"
	"code.cloudfoundry.org/korifi/api/routing"

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
	roleDeletePrefix   = "role.delete"

	JobTimeoutDuration = 120.0
)

const JobResourceType = "Job"

type Job struct {
	serverURL       url.URL
	orgRepo         CFOrgRepository
	spaceRepo       CFSpaceRepository
	pollingInterval time.Duration
}

func NewJob(serverURL url.URL, orgRepo CFOrgRepository, spaceRepo CFSpaceRepository, pollingInterval time.Duration) *Job {
	return &Job{
		serverURL:       serverURL,
		orgRepo:         orgRepo,
		spaceRepo:       spaceRepo,
		pollingInterval: pollingInterval,
	}
}

func (h *Job) get(r *http.Request) (*routing.Response, error) {
	log := logr.FromContextOrDiscard(r.Context()).WithName("handlers.job.get")

	jobGUID := routing.URLParam(r, "guid")

	jobType, resourceGUID, match := parseJobGUID(jobGUID)

	if !match {
		return nil, apierrors.LogAndReturn(
			log,
			apierrors.NewNotFoundError(fmt.Errorf("invalid job guid: %s", jobGUID), JobResourceType),
			"Invalid Job GUID",
		)
	}

	var (
		err         error
		jobResponse presenter.JobResponse
	)

	switch jobType {
	case syncSpacePrefix:
		jobResponse = presenter.ForManifestApplyJob(jobGUID, resourceGUID, h.serverURL)
	case appDeletePrefix, routeDeletePrefix, domainDeletePrefix, roleDeletePrefix:
		jobResponse = presenter.ForJob(jobGUID, []presenter.JobResponseError{}, presenter.StateComplete, jobType, h.serverURL)
	case orgDeletePrefix, spaceDeletePrefix:
		jobResponse, err = h.handleDeleteJob(r.Context(), jobType, resourceGUID)
		if err != nil {
			return nil, err
		}
	default:
		return nil, apierrors.LogAndReturn(
			log,
			apierrors.NewNotFoundError(fmt.Errorf("invalid job type: %s", jobType), JobResourceType),
			fmt.Sprintf("Invalid Job type: %s", jobType),
		)
	}

	return routing.NewResponse(http.StatusOK).WithBody(jobResponse), nil
}

func (h *Job) handleDeleteJob(ctx context.Context, jobType, resourceGUID string) (presenter.JobResponse, error) {
	authInfo, _ := authorization.InfoFromContext(ctx)
	jobGUID := jobType + presenter.JobGUIDDelimiter + resourceGUID
	log := logr.FromContextOrDiscard(ctx).WithName("handlers.job.get.handleDeleteJob")

	var (
		org          repositories.OrgRecord
		space        repositories.SpaceRecord
		err          error
		resourceType string
		deletedAt    *time.Time
	)

	for retries := 0; retries < 40; retries++ {
		switch jobType {
		case orgDeletePrefix:
			org, err = h.orgRepo.GetOrgUnfiltered(ctx, authInfo, resourceGUID)
			resourceType = "Org"
			deletedAt = org.DeletedAt
		case spaceDeletePrefix:
			space, err = h.spaceRepo.GetSpace(ctx, authInfo, resourceGUID)
			resourceType = "Space"
			deletedAt = space.DeletedAt
		}

		if err != nil {
			switch err.(type) {
			case apierrors.NotFoundError, apierrors.ForbiddenError:
				return presenter.ForJob(jobGUID,
					[]presenter.JobResponseError{},
					presenter.StateComplete,
					jobType,
					h.serverURL,
				), nil
			default:
				return presenter.JobResponse{}, apierrors.LogAndReturn(
					log,
					err,
					"failed to fetch "+resourceType+" from Kubernetes",
					resourceType+"GUID", resourceGUID,
				)
			}
		}

		if deletedAt != nil {
			break
		}

		log.V(1).Info("Waiting for deletion timestamp", resourceType+"GUID", resourceGUID)
		time.Sleep(h.pollingInterval)
	}

	return h.handleDeleteJobResponse(ctx, deletedAt, jobType, resourceGUID, resourceType)
}

func (h *Job) handleDeleteJobResponse(ctx context.Context, deletedAt *time.Time, jobType, resourceGUID, resourceType string) (presenter.JobResponse, error) {
	jobGUID := jobType + presenter.JobGUIDDelimiter + resourceGUID
	log := logr.FromContextOrDiscard(ctx).WithName("handlers.job.get.handleDeleteJobResponse")

	if deletedAt == nil {
		return presenter.JobResponse{}, apierrors.LogAndReturn(
			log,
			apierrors.NewNotFoundError(fmt.Errorf("job %q not found", jobGUID), JobResourceType),
			resourceType+" not marked for deletion",
			resourceType+"GUID", resourceGUID,
		)
	}

	if time.Since(*deletedAt).Seconds() < JobTimeoutDuration {
		return presenter.ForJob(
			jobGUID,
			[]presenter.JobResponseError{},
			presenter.StateProcessing,
			jobType,
			h.serverURL,
		), nil
	}

	return presenter.ForJob(
		jobGUID,
		[]presenter.JobResponseError{{
			Code:   10008,
			Detail: fmt.Sprintf("%s deletion timed out. Check for remaining resources in the %q namespace", resourceType, resourceGUID),
			Title:  "CF-UnprocessableEntity",
		}},
		presenter.StateFailed,
		jobType,
		h.serverURL,
	), nil
}

func (h *Job) UnauthenticatedRoutes() []routing.Route {
	return nil
}

func (h *Job) AuthenticatedRoutes() []routing.Route {
	return []routing.Route{
		{Method: "GET", Pattern: JobPath, Handler: h.get},
	}
}

var (
	jobOperationPattern       = `([a-z_\-]+\.[a-z_]+)` // (e.g. app.delete, space.apply_manifest, etc.)
	resourceIdentifierPattern = `([A-Za-z0-9\-\.]+)`   // (e.g. cf-space-a4cd478b-0b02-452f-8498-ce87ec5c6649, CUSTOM_ORG_ID, etc.)
	jobRegexp                 = regexp.MustCompile(jobOperationPattern + presenter.JobGUIDDelimiter + resourceIdentifierPattern)
)

func parseJobGUID(jobGUID string) (string, string, bool) {
	// Parse the job identifier and capture the job operation and resource name for later use
	matches := jobRegexp.FindStringSubmatch(jobGUID)

	if len(matches) != 3 {
		return "", "", false
	} else {
		return matches[1], matches[2], true
	}
}
