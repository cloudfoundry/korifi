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
	"code.cloudfoundry.org/korifi/api/routing"
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/tools/logger"
)

const (
	JobPath                      = "/v3/jobs/{guid}"
	syncSpaceJobType             = "space.apply_manifest"
	AppDeleteJobType             = "app.delete"
	OrgDeleteJobType             = "org.delete"
	RouteDeleteJobType           = "route.delete"
	SpaceDeleteJobType           = "space.delete"
	DomainDeleteJobType          = "domain.delete"
	RoleDeleteJobType            = "role.delete"
	BrokerCreateJobType          = "service_broker.create"
	ServiceInstanceCreateJobType = "service_instance.create"
	OrgQuotaDeleteJobType        = "orgquota.delete"
	SpaceQuotaDeleteJobType      = "spacequota.delete"

	JobTimeoutDuration = 120.0
)

const JobResourceType = "Job"

//counterfeiter:generate -o fake -fake-name DeletionRepository . DeletionRepository
type DeletionRepository interface {
	GetDeletedAt(context.Context, authorization.Info, string) (*time.Time, error)
}

type Job struct {
	serverURL                 url.URL
	repositories              map[string]DeletionRepository
	brokerRepository          CFServiceBrokerRepository
	serviceInstanceRepository CFServiceInstanceRepository
	pollingInterval           time.Duration
}

func NewJob(
	serverURL url.URL,
	repositories map[string]DeletionRepository,
	brokerRepository CFServiceBrokerRepository,
	serviceInstanceRepository CFServiceInstanceRepository,
	pollingInterval time.Duration,
) *Job {
	return &Job{
		serverURL:                 serverURL,
		repositories:              repositories,
		brokerRepository:          brokerRepository,
		serviceInstanceRepository: serviceInstanceRepository,
		pollingInterval:           pollingInterval,
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

	if job.Type == syncSpaceJobType {
		return routing.NewResponse(http.StatusOK).WithBody(presenter.ForManifestApplyJob(job, h.serverURL)), nil
	}

	authInfo, _ := authorization.InfoFromContext(ctx)
	if job.Type == BrokerCreateJobType {
		broker, err := h.brokerRepository.GetServiceBroker(ctx, authInfo, job.ResourceGUID)
		if err != nil {
			return nil, apierrors.LogAndReturn(log, err, "getting broker failed")
		}

		state := presenter.StateProcessing
		if broker.IsReady() {
			state = presenter.StateComplete
		}
		return routing.NewResponse(http.StatusOK).WithBody(presenter.ForJob(job, []presenter.JobResponseError{}, state, h.serverURL)), nil
	}

	if job.Type == ServiceInstanceCreateJobType {
		serviceInstance, err := h.serviceInstanceRepository.GetServiceInstance(ctx, authInfo, job.ResourceGUID)
		if err != nil {
			return nil, apierrors.LogAndReturn(log, err, "getting service instance failed")
		}

		if serviceInstance.State == nil {
			return routing.NewResponse(http.StatusOK).WithBody(presenter.ForJob(job, []presenter.JobResponseError{}, presenter.StateProcessing, h.serverURL)), nil
		}

		if serviceInstance.State.Status == korifiv1alpha1.FailedStatus {
			failedResponse := presenter.ForJob(
				job,
				[]presenter.JobResponseError{{
					Code:   10008,
					Detail: fmt.Sprintf("service instance %q creation failed: %s", job.ResourceGUID, serviceInstance.State.Details),
					Title:  "CF-UnprocessableEntity",
				}},
				presenter.StateFailed,
				h.serverURL,
			)
			return routing.NewResponse(http.StatusOK).WithBody(failedResponse), nil
		}
		if serviceInstance.State.Status == korifiv1alpha1.ReadyStatus {
			return routing.NewResponse(http.StatusOK).WithBody(presenter.ForJob(job, []presenter.JobResponseError{}, presenter.StateComplete, h.serverURL)), nil
		}
	}

	repository, ok := h.repositories[job.Type]
	if !ok {
		return nil, apierrors.LogAndReturn(
			log,
			apierrors.NewNotFoundError(fmt.Errorf("invalid job type: %s", job.Type), JobResourceType),
			fmt.Sprintf("Invalid Job type: %s", job.Type),
		)
	}

	jobResponse, err := h.handleDeleteJob(ctx, repository, job)
	if err != nil {
		return nil, err
	}

	return routing.NewResponse(http.StatusOK).WithBody(jobResponse), nil
}

func (h *Job) handleDeleteJob(ctx context.Context, repository DeletionRepository, job presenter.Job) (presenter.JobResponse, error) {
	ctx, log := logger.FromContext(ctx, "handleDeleteJob")

	deletedAt, err := h.retryGetDeletedAt(ctx, repository, job)
	if err != nil {
		if errors.As(err, &apierrors.NotFoundError{}) || errors.As(err, &apierrors.ForbiddenError{}) {
			return presenter.ForJob(job,
				[]presenter.JobResponseError{},
				presenter.StateComplete,
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
