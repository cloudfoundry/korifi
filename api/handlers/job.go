package handlers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"code.cloudfoundry.org/korifi/api/authorization"
	apierrors "code.cloudfoundry.org/korifi/api/errors"
	"code.cloudfoundry.org/korifi/api/presenter"
	"code.cloudfoundry.org/korifi/api/repositories"
	"code.cloudfoundry.org/korifi/api/routing"
	"code.cloudfoundry.org/korifi/tools/logger"
)

const (
	JobPath                             = "/v3/jobs/{guid}"
	syncSpaceJobType                    = "space.apply_manifest"
	spaceDeleteUnmappedRoutesJobType    = "space.delete_unapped_routes"
	AppDeleteJobType                    = "app.delete"
	OrgDeleteJobType                    = "org.delete"
	RouteDeleteJobType                  = "route.delete"
	SpaceDeleteJobType                  = "space.delete"
	DomainDeleteJobType                 = "domain.delete"
	RoleDeleteJobType                   = "role.delete"
	ServiceBrokerCreateJobType          = "service_broker.create"
	ServiceBrokerUpdateJobType          = "service_broker.update"
	ServiceBrokerDeleteJobType          = "service_broker.delete"
	ManagedServiceInstanceDeleteJobType = "managed_service_instance.delete"
	ManagedServiceInstanceCreateJobType = "managed_service_instance.create"
	ManagedServiceBindingCreateJobType  = "managed_service_binding.create"
	ManagedServiceBindingDeleteJobType  = "managed_service_binding.delete"
	JobTimeoutDuration                  = 120.0
)

const JobResourceType = "Job"

//counterfeiter:generate -o fake -fake-name DeletionRepository . DeletionRepository
type DeletionRepository interface {
	GetDeletedAt(context.Context, authorization.Info, string) (*time.Time, error)
}

//counterfeiter:generate -o fake -fake-name StateRepository . StateRepository
type StateRepository interface {
	GetState(context.Context, authorization.Info, string) (repositories.ResourceState, error)
}

type Job struct {
	serverURL            url.URL
	deletionRepositories map[string]DeletionRepository
	stateRepositories    map[string]StateRepository
	routeRepo            CFRouteRepository
	pollingInterval      time.Duration
}

func NewJob(
	serverURL url.URL,
	deletionRepositories map[string]DeletionRepository,
	stateRepositories map[string]StateRepository,
	routeRepo CFRouteRepository,
	pollingInterval time.Duration,
) *Job {
	return &Job{
		serverURL:            serverURL,
		deletionRepositories: deletionRepositories,
		stateRepositories:    stateRepositories,
		routeRepo:            routeRepo,
		pollingInterval:      pollingInterval,
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

	switch job.Type {
	case syncSpaceJobType:
		return routing.NewResponse(http.StatusOK).WithBody(presenter.ForManifestApplyJob(job, h.serverURL)), nil
	case spaceDeleteUnmappedRoutesJobType:
		authInfo, _ := authorization.InfoFromContext(ctx)
		state := repositories.ResourceStateReady

		unmappedRoutes, err := h.routeRepo.ListRoutes(ctx, authInfo, repositories.ListRoutesMessage{SpaceGUIDs: []string{job.ResourceGUID}, IsUnmapped: true})
		if err != nil {
			return nil, err
		}

		if len(unmappedRoutes) != 0 {
			state = repositories.ResourceStateUnknown
		}

		return routing.NewResponse(http.StatusOK).WithBody(presenter.ForSpaceDeleteUnmappedRoutesJob(job, state, h.serverURL)), nil

	default:
		deletionRepository, ok := h.deletionRepositories[job.Type]
		if ok {
			jobResponse, err := h.handleDeleteJob(ctx, deletionRepository, job)
			if err != nil {
				return nil, err
			}

			return routing.NewResponse(http.StatusOK).WithBody(jobResponse), nil
		}

		stateRepository, ok := h.stateRepositories[job.Type]
		if ok {
			jobResponse, err := h.handleStateJob(ctx, stateRepository, job)
			if err != nil {
				return nil, err
			}

			return routing.NewResponse(http.StatusOK).WithBody(jobResponse), nil
		}

		return nil, apierrors.LogAndReturn(
			log,
			apierrors.NewNotFoundError(fmt.Errorf("invalid job type: %s", job.Type), JobResourceType),
			fmt.Sprintf("Invalid Job type: %s", job.Type),
		)

	}
}

func (h *Job) handleDeleteJob(ctx context.Context, repository DeletionRepository, job presenter.Job) (presenter.JobResponse, error) {
	ctx, log := logger.FromContext(ctx, "handleDeleteJob")

	deletedAt, err := h.retryGetDeletedAt(ctx, repository, job)
	if err != nil {
		if errors.As(err, &apierrors.NotFoundError{}) || errors.As(err, &apierrors.ForbiddenError{}) {
			return presenter.ForJob(job,
				[]presenter.JobResponseError{},
				repositories.ResourceStateReady,
				h.serverURL,
			), nil
		}

		return presenter.JobResponse{}, apierrors.LogAndReturn(
			log,
			err,
			"failed to fetch "+job.ResourceType+" from Kubernetes",
			job.ResourceType+"GUID", job.ResourceGUID,
		)
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
			repositories.ResourceStateUnknown,
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
		repositories.ResourceStateUnknown,
		h.serverURL,
	), nil
}

func (h *Job) handleStateJob(ctx context.Context, repository StateRepository, job presenter.Job) (presenter.JobResponse, error) {
	ctx, log := logger.FromContext(ctx, "handleStateJob")
	authInfo, _ := authorization.InfoFromContext(ctx)
	state, err := repository.GetState(ctx, authInfo, job.ResourceGUID)
	if err != nil {
		if errors.As(err, &apierrors.ForbiddenError{}) {
			return presenter.ForJob(job,
				[]presenter.JobResponseError{},
				repositories.ResourceStateReady,
				h.serverURL,
			), nil
		}
		return presenter.JobResponse{}, apierrors.LogAndReturn(
			log,
			err,
			"failed to get "+job.ResourceType+" state from Kubernetes",
			job.ResourceType+"GUID", job.ResourceGUID,
		)
	}

	return presenter.ForJob(job,
		[]presenter.JobResponseError{},
		state,
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
