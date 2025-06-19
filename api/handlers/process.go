package handlers

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"code.cloudfoundry.org/korifi/api/authorization"
	apierrors "code.cloudfoundry.org/korifi/api/errors"
	"code.cloudfoundry.org/korifi/api/handlers/stats"
	"code.cloudfoundry.org/korifi/api/payloads"
	"code.cloudfoundry.org/korifi/api/presenter"
	"code.cloudfoundry.org/korifi/api/repositories"
	"code.cloudfoundry.org/korifi/api/routing"

	"github.com/go-logr/logr"
)

const (
	ProcessPath                = "/v3/processes/{guid}"
	ProcessSidecarsPath        = "/v3/processes/{guid}/sidecars"
	ProcessScalePath           = "/v3/processes/{guid}/actions/scale"
	ProcessStatsPath           = "/v3/processes/{guid}/stats"
	ProcessesPath              = "/v3/processes"
	ProcessInstanceRestartPath = "/v3/processes/{guid}/instances/{instanceID}"
)

//counterfeiter:generate -o fake -fake-name CFProcessRepository . CFProcessRepository
type CFProcessRepository interface {
	GetProcess(context.Context, authorization.Info, string) (repositories.ProcessRecord, error)
	ListProcesses(context.Context, authorization.Info, repositories.ListProcessesMessage) (repositories.ListResult[repositories.ProcessRecord], error)
	GetAppRevision(ctx context.Context, authInfo authorization.Info, appGUID string) (string, error)
	PatchProcess(context.Context, authorization.Info, repositories.PatchProcessMessage) (repositories.ProcessRecord, error)
	CreateProcess(context.Context, authorization.Info, repositories.CreateProcessMessage) error
	ScaleProcess(ctx context.Context, authInfo authorization.Info, scaleProcessMessage repositories.ScaleProcessMessage) (repositories.ProcessRecord, error)
}

//counterfeiter:generate -o fake -fake-name GaugesCollector . GaugesCollector
type GaugesCollector interface {
	CollectProcessGauges(ctx context.Context, appGUID, processGUID string) ([]stats.ProcessGauges, error)
}

//counterfeiter:generate -o fake -fake-name InstancesStateCollector . InstancesStateCollector
type InstancesStateCollector interface {
	CollectProcessInstancesStates(ctx context.Context, processGUID string) ([]stats.ProcessInstanceState, error)
}

type Process struct {
	serverURL               url.URL
	processRepo             CFProcessRepository
	requestValidator        RequestValidator
	podRepo                 PodRepository
	gaugesCollector         GaugesCollector
	instancesStateCollector InstancesStateCollector
}

func NewProcess(
	serverURL url.URL,
	processRepo CFProcessRepository,
	requestValidator RequestValidator,
	podRepo PodRepository,
	gaugesCollector GaugesCollector,
	instancesStateCollector InstancesStateCollector,
) *Process {
	return &Process{
		serverURL:               serverURL,
		processRepo:             processRepo,
		requestValidator:        requestValidator,
		podRepo:                 podRepo,
		gaugesCollector:         gaugesCollector,
		instancesStateCollector: instancesStateCollector,
	}
}

func (h *Process) get(r *http.Request) (*routing.Response, error) {
	authInfo, _ := authorization.InfoFromContext(r.Context())
	logger := logr.FromContextOrDiscard(r.Context()).WithName("handlers.process.get")

	processGUID := routing.URLParam(r, "guid")

	process, err := h.processRepo.GetProcess(r.Context(), authInfo, processGUID)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, apierrors.ForbiddenAsNotFound(err), "Failed to fetch process from Kubernetes", "ProcessGUID", processGUID)
	}

	return routing.NewResponse(http.StatusOK).WithBody(presenter.ForProcess(process, h.serverURL)), nil
}

func (h *Process) restartProcessInstance(r *http.Request) (*routing.Response, error) {
	authInfo, _ := authorization.InfoFromContext(r.Context())
	logger := logr.FromContextOrDiscard(r.Context()).WithName("handlers.process.delete")
	processGUID := routing.URLParam(r, "guid")
	instanceID := routing.URLParam(r, "instanceID")

	process, err := h.processRepo.GetProcess(r.Context(), authInfo, processGUID)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, apierrors.NewNotFoundError(nil, repositories.ProcessResourceType), "Failed to fetch process", "ProcessGUID", processGUID)
	}
	if process.AppGUID == "" {
		return nil, apierrors.LogAndReturn(logger, apierrors.NewNotFoundError(nil, repositories.ProcessResourceType), "Failed to fetch appGUID via process", "ProcessGUID", processGUID)
	}

	appRevision, err := h.processRepo.GetAppRevision(r.Context(), authInfo, process.AppGUID)
	if err != nil {
		return nil, apierrors.LogAndReturn(
			logger,
			apierrors.AsUnprocessableEntity(err, "Cannot get App Revision. Ensure that the process exists and you have access to it.", apierrors.NotFoundError{}, apierrors.ForbiddenError{}),
			"Process GUID", processGUID,
			"AppGUID", process.AppGUID,
		)
	}

	instance, err := strconv.Atoi(instanceID)
	if err != nil {
		return nil, apierrors.LogAndReturn(
			logger,
			apierrors.AsUnprocessableEntity(err, "Invalid Instance ID. Instance ID is not a valid Integer.", apierrors.NotFoundError{}, apierrors.ForbiddenError{}),
			"InstanceID", instanceID,
		)
	}
	if int(process.DesiredInstances) <= instance {
		return nil, apierrors.LogAndReturn(logger,
			apierrors.NewNotFoundError(nil, fmt.Sprintf("Instance %d of process %s", instance, process.Type)), "Instance not found", "AppGUID", process.AppGUID, "InstanceID", instanceID, "Process", process.Type)
	}

	err = h.podRepo.DeletePod(r.Context(), authInfo, appRevision, process, instanceID)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, apierrors.ForbiddenAsNotFound(err), "Failed to restart instance", "AppGUID", process.AppGUID, "InstanceID", instanceID, "Process", process.Type)
	}

	return routing.NewResponse(http.StatusNoContent), nil
}

func (h *Process) getSidecars(r *http.Request) (*routing.Response, error) {
	authInfo, _ := authorization.InfoFromContext(r.Context())
	logger := logr.FromContextOrDiscard(r.Context()).WithName("handlers.process.get-sidecars")

	processGUID := routing.URLParam(r, "guid")

	_, err := h.processRepo.GetProcess(r.Context(), authInfo, processGUID)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, apierrors.ForbiddenAsNotFound(err), "Failed to fetch process from Kubernetes", "ProcessGUID", processGUID)
	}

	return routing.NewResponse(http.StatusOK).WithBody(map[string]interface{}{
		"pagination": map[string]interface{}{
			"total_results": 0,
			"total_pages":   1,
			"first": map[string]interface{}{
				"href": fmt.Sprintf("%s/v3/processes/%s/sidecars", h.serverURL.String(), processGUID),
			},
			"last": map[string]interface{}{
				"href": fmt.Sprintf("%s/v3/processes/%s/sidecars", h.serverURL.String(), processGUID),
			},
			"next":     nil,
			"previous": nil,
		},
		"resources": []string{},
	}), nil
}

func (h *Process) scale(r *http.Request) (*routing.Response, error) {
	authInfo, _ := authorization.InfoFromContext(r.Context())
	logger := logr.FromContextOrDiscard(r.Context()).WithName("handlers.process.scale")

	processGUID := routing.URLParam(r, "guid")

	var payload payloads.ProcessScale
	if err := h.requestValidator.DecodeAndValidateJSONPayload(r, &payload); err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "failed to decode payload")
	}

	process, err := h.processRepo.GetProcess(r.Context(), authInfo, processGUID)
	if err != nil {
		return nil, apierrors.ForbiddenAsNotFound(err)
	}

	processRecord, err := h.processRepo.ScaleProcess(r.Context(), authInfo, repositories.ScaleProcessMessage{
		GUID:               process.GUID,
		SpaceGUID:          process.SpaceGUID,
		ProcessScaleValues: payload.ToRecord(),
	})
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "failed to scale process", "processGUID", processGUID)
	}

	return routing.NewResponse(http.StatusOK).WithBody(presenter.ForProcess(processRecord, h.serverURL)), nil
}

func (h *Process) getStats(r *http.Request) (*routing.Response, error) {
	processGUID := routing.URLParam(r, "guid")
	authInfo, _ := authorization.InfoFromContext(r.Context())
	logger := logr.FromContextOrDiscard(r.Context()).WithName("handlers.process.get-stats").WithValues("processGUID", processGUID)

	process, err := h.processRepo.GetProcess(r.Context(), authInfo, processGUID)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, apierrors.ForbiddenAsNotFound(err), "failed to get process from Kubernetes")
	}

	gauges, err := h.gaugesCollector.CollectProcessGauges(r.Context(), process.AppGUID, process.GUID)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "failed to get process gauges from log cache")
	}

	instancesState, err := h.instancesStateCollector.CollectProcessInstancesStates(r.Context(), processGUID)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "failed to get process instances state")
	}

	return routing.NewResponse(http.StatusOK).WithBody(presenter.ForProcessStats(gauges, instancesState, time.Now())), nil
}

func (h *Process) list(r *http.Request) (*routing.Response, error) { //nolint:dupl
	authInfo, _ := authorization.InfoFromContext(r.Context())
	logger := logr.FromContextOrDiscard(r.Context()).WithName("handlers.process.list")

	processListFilter := new(payloads.ProcessList)
	err := h.requestValidator.DecodeAndValidateURLValues(r, processListFilter)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "Unable to decode request query parameters")
	}

	processList, err := h.processRepo.ListProcesses(r.Context(), authInfo, processListFilter.ToMessage())
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "Failed to fetch processes(s) from Kubernetes")
	}

	return routing.NewResponse(http.StatusOK).WithBody(presenter.ForList(presenter.ForProcessSanitized, processList, h.serverURL, *r.URL)), nil
}

func (h *Process) update(r *http.Request) (*routing.Response, error) {
	authInfo, _ := authorization.InfoFromContext(r.Context())
	logger := logr.FromContextOrDiscard(r.Context()).WithName("handlers.process.update")

	processGUID := routing.URLParam(r, "guid")

	var payload payloads.ProcessPatch
	if err := h.requestValidator.DecodeAndValidateJSONPayload(r, &payload); err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "failed to decode json payload")
	}

	process, err := h.processRepo.GetProcess(r.Context(), authInfo, processGUID)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, apierrors.ForbiddenAsNotFound(err), "Failed to get process from Kubernetes", "ProcessGUID", processGUID)
	}

	updatedProcess, err := h.processRepo.PatchProcess(r.Context(), authInfo, payload.ToProcessPatchMessage(processGUID, process.SpaceGUID))
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "Failed to patch process from Kubernetes", "ProcessGUID", processGUID)
	}

	return routing.NewResponse(http.StatusOK).WithBody(presenter.ForProcess(updatedProcess, h.serverURL)), nil
}

func (h *Process) UnauthenticatedRoutes() []routing.Route {
	return nil
}

func (h *Process) AuthenticatedRoutes() []routing.Route {
	return []routing.Route{
		{Method: "GET", Pattern: ProcessPath, Handler: h.get},
		{Method: "GET", Pattern: ProcessSidecarsPath, Handler: h.getSidecars},
		{Method: "POST", Pattern: ProcessScalePath, Handler: h.scale},
		{Method: "GET", Pattern: ProcessStatsPath, Handler: h.getStats},
		{Method: "GET", Pattern: ProcessesPath, Handler: h.list},
		{Method: "PATCH", Pattern: ProcessPath, Handler: h.update},
		{Method: "DELETE", Pattern: ProcessInstanceRestartPath, Handler: h.restartProcessInstance},
	}
}
