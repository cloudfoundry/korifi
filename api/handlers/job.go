package handlers

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
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

	job, match := presenter.JobFromGUID(jobGUID)
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

	switch job.Type {
	case syncSpacePrefix:
		jobResponse = presenter.ForManifestApplyJob(job, h.serverURL)
	case appDeletePrefix, routeDeletePrefix, domainDeletePrefix, roleDeletePrefix:
		jobResponse = presenter.ForJob(job, []presenter.JobResponseError{}, presenter.StateComplete, h.serverURL)
	case orgDeletePrefix, spaceDeletePrefix:
		jobResponse, err = h.handleDeleteJob(r.Context(), job)
		if err != nil {
			return nil, err
		}
	default:
		return nil, apierrors.LogAndReturn(
			log,
			apierrors.NewNotFoundError(fmt.Errorf("invalid job type: %s", job.Type), JobResourceType),
			fmt.Sprintf("Invalid Job type: %s", job.Type),
		)
	}

	return routing.NewResponse(http.StatusOK).WithBody(jobResponse), nil
}

func (h *Job) handleDeleteJob(ctx context.Context, job presenter.Job) (presenter.JobResponse, error) {
	authInfo, _ := authorization.InfoFromContext(ctx)
	log := logr.FromContextOrDiscard(ctx).WithName("handlers.job.get.handleDeleteJob")

	var (
		org       repositories.OrgRecord
		space     repositories.SpaceRecord
		err       error
		deletedAt *time.Time
	)

	for retries := 0; retries < 40; retries++ {
		switch job.Type {
		case orgDeletePrefix:
			org, err = h.orgRepo.GetOrgUnfiltered(ctx, authInfo, job.ResourceGUID)
			deletedAt = org.DeletedAt
		case spaceDeletePrefix:
			space, err = h.spaceRepo.GetSpace(ctx, authInfo, job.ResourceGUID)
			deletedAt = space.DeletedAt
		}

		if err != nil {
			switch err.(type) {
			case apierrors.NotFoundError, apierrors.ForbiddenError:
				return presenter.ForJob(job,
					[]presenter.JobResponseError{},
					presenter.StateComplete,
					h.serverURL,
				), nil
			default:
				return presenter.JobResponse{}, apierrors.LogAndReturn(
					log,
					err,
					"failed to fetch "+job.ResourceType+" from Kubernetes",
					job.ResourceType+"GUID", job.ResourceGUID,
				)
			}
		}

		if deletedAt != nil {
			break
		}

		log.V(1).Info("Waiting for deletion timestamp", job.ResourceType+"GUID", job.ResourceGUID)
		time.Sleep(h.pollingInterval)
	}

	return h.handleDeleteJobResponse(ctx, deletedAt, job)
}

func (h *Job) handleDeleteJobResponse(ctx context.Context, deletedAt *time.Time, job presenter.Job) (presenter.JobResponse, error) {
	log := logr.FromContextOrDiscard(ctx).WithName("handlers.job.get.handleDeleteJobResponse")

	if deletedAt == nil {
		return presenter.JobResponse{}, apierrors.LogAndReturn(
			log,
			apierrors.NewNotFoundError(fmt.Errorf("job %q not found", job.GUID), JobResourceType),
			job.ResourceType+" not marked for deletion",
			job.ResourceType+"GUID", job.GUID,
		)
	}

	if time.Since(*deletedAt).Seconds() < JobTimeoutDuration {
		return presenter.ForJob(
			job,
			[]presenter.JobResponseError{},
			presenter.StateProcessing,
			h.serverURL,
		), nil
	}

	return presenter.ForJob(
		job,
		[]presenter.JobResponseError{{
			Code:   10008,
			Detail: fmt.Sprintf("%s deletion timed out. Check for remaining resources in the %q namespace", job.ResourceType, job.ResourceGUID),
			Title:  "CF-UnprocessableEntity",
		}},
		presenter.StateFailed,
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
