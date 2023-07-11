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
	"code.cloudfoundry.org/korifi/api/routing"
	"code.cloudfoundry.org/korifi/tools/logger"
)

const (
	JobPath             = "/v3/jobs/{guid}"
	syncSpaceJobType    = "space.apply_manifest"
	AppDeleteJobType    = "app.delete"
	OrgDeleteJobType    = "org.delete"
	RouteDeleteJobType  = "route.delete"
	SpaceDeleteJobType  = "space.delete"
	DomainDeleteJobType = "domain.delete"
	RoleDeleteJobType   = "role.delete"

	JobTimeoutDuration = 120.0
)

const JobResourceType = "Job"

//counterfeiter:generate -o fake -fake-name DeletionRepository . DeletionRepository
type DeletionRepository interface {
	GetDeletedAt(context.Context, authorization.Info, string) (*time.Time, error)
}

type Job struct {
	serverURL       url.URL
	repositories    map[string]DeletionRepository
	pollingInterval time.Duration
}

func NewJob(serverURL url.URL, repositories map[string]DeletionRepository, pollingInterval time.Duration) *Job {
	return &Job{
		serverURL:       serverURL,
		repositories:    repositories,
		pollingInterval: pollingInterval,
	}
}

func (h *Job) get(r *http.Request) (*routing.Response, error) {
	ctx, log := logger.FromContext(r.Context(), "handlers.job.get")

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
	case syncSpaceJobType:
		jobResponse = presenter.ForManifestApplyJob(job, h.serverURL)
	case RouteDeleteJobType, DomainDeleteJobType, RoleDeleteJobType:
		jobResponse = presenter.ForJob(job, []presenter.JobResponseError{}, presenter.StateComplete, h.serverURL)
	default:
		repository, ok := h.repositories[job.Type]
		if !ok {
			return nil, apierrors.LogAndReturn(
				log,
				apierrors.NewNotFoundError(fmt.Errorf("invalid job type: %s", job.Type), JobResourceType),
				fmt.Sprintf("Invalid Job type: %s", job.Type),
			)
		}

		jobResponse, err = h.handleDeleteJob(ctx, repository, job)
		if err != nil {
			return nil, err
		}
	}

	return routing.NewResponse(http.StatusOK).WithBody(jobResponse), nil
}

func (h *Job) handleDeleteJob(ctx context.Context, repository DeletionRepository, job presenter.Job) (presenter.JobResponse, error) {
	ctx, log := logger.FromContext(ctx, "handleDeleteJob")

	deletedAt, err := h.retryGetDeletedAt(ctx, repository, job)
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
			Detail: fmt.Sprintf("%s deletion timed out, check the remaining %q resource", job.ResourceType, job.ResourceGUID),
			Title:  "CF-UnprocessableEntity",
		}},
		presenter.StateFailed,
		h.serverURL,
	), nil
}

func (h *Job) retryGetDeletedAt(ctx context.Context, repository DeletionRepository, job presenter.Job) (*time.Time, error) {
	ctx, log := logger.FromContext(ctx, "retryGetDeletedAt")
	authInfo, _ := authorization.InfoFromContext(ctx)

	var (
		deletedAt *time.Time
		err       error
	)

	for retries := 0; retries < 40; retries++ {
		deletedAt, err = repository.GetDeletedAt(ctx, authInfo, job.ResourceGUID)
		if err != nil {
			return nil, err
		}

		if deletedAt != nil {
			return deletedAt, nil
		}

		log.V(1).Info("Waiting for deletion timestamp", job.ResourceType+"GUID", job.ResourceGUID)
		time.Sleep(h.pollingInterval)
	}

	return nil, nil
}

func (h *Job) UnauthenticatedRoutes() []routing.Route {
	return nil
}

func (h *Job) AuthenticatedRoutes() []routing.Route {
	return []routing.Route{
		{Method: "GET", Pattern: JobPath, Handler: h.get},
	}
}
