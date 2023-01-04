package handlers

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	"code.cloudfoundry.org/korifi/api/apierrors"
	"code.cloudfoundry.org/korifi/api/authorization"
	"code.cloudfoundry.org/korifi/api/payloads"
	"code.cloudfoundry.org/korifi/api/presenter"
	"code.cloudfoundry.org/korifi/api/repositories"
	"code.cloudfoundry.org/korifi/api/routing"

	"github.com/go-chi/chi"
	"github.com/go-logr/logr"
)

const (
	ProcessPath         = "/v3/processes/{guid}"
	ProcessSidecarsPath = "/v3/processes/{guid}/sidecars"
	ProcessScalePath    = "/v3/processes/{guid}/actions/scale"
	ProcessStatsPath    = "/v3/processes/{guid}/stats"
	ProcessesPath       = "/v3/processes"
)

//counterfeiter:generate -o fake -fake-name CFProcessRepository . CFProcessRepository
type CFProcessRepository interface {
	GetProcess(context.Context, authorization.Info, string) (repositories.ProcessRecord, error)
	ListProcesses(context.Context, authorization.Info, repositories.ListProcessesMessage) ([]repositories.ProcessRecord, error)
	GetProcessByAppTypeAndSpace(context.Context, authorization.Info, string, string, string) (repositories.ProcessRecord, error)
	PatchProcess(context.Context, authorization.Info, repositories.PatchProcessMessage) (repositories.ProcessRecord, error)
}

//counterfeiter:generate -o fake -fake-name ProcessScaler . ProcessScaler
type ProcessScaler interface {
	ScaleProcess(ctx context.Context, authInfo authorization.Info, processGUID string, scale repositories.ProcessScaleValues) (repositories.ProcessRecord, error)
}

//counterfeiter:generate -o fake -fake-name ProcessStatsFetcher . ProcessStatsFetcher
type ProcessStatsFetcher interface {
	FetchStats(context.Context, authorization.Info, string) ([]repositories.PodStatsRecord, error)
}

type ProcessHandler struct {
	serverURL           url.URL
	processRepo         CFProcessRepository
	processStatsFetcher ProcessStatsFetcher
	processScaler       ProcessScaler
	decoderValidator    *DecoderValidator
}

func NewProcessHandler(
	serverURL url.URL,
	processRepo CFProcessRepository,
	processStatsFetcher ProcessStatsFetcher,
	scaleProcessFunc ProcessScaler,
	decoderValidator *DecoderValidator,
) *ProcessHandler {
	return &ProcessHandler{
		serverURL:           serverURL,
		processRepo:         processRepo,
		processStatsFetcher: processStatsFetcher,
		processScaler:       scaleProcessFunc,
		decoderValidator:    decoderValidator,
	}
}

func (h *ProcessHandler) processGetHandler(r *http.Request) (*routing.Response, error) {
	authInfo, _ := authorization.InfoFromContext(r.Context())
	logger := logr.FromContextOrDiscard(r.Context()).WithName("process-handler.process-get")

	processGUID := chi.URLParam(r, "guid")

	process, err := h.processRepo.GetProcess(r.Context(), authInfo, processGUID)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, apierrors.ForbiddenAsNotFound(err), "Failed to fetch process from Kubernetes", "ProcessGUID", processGUID)
	}

	return routing.NewHandlerResponse(http.StatusOK).WithBody(presenter.ForProcess(process, h.serverURL)), nil
}

func (h *ProcessHandler) processGetSidecarsHandler(r *http.Request) (*routing.Response, error) {
	authInfo, _ := authorization.InfoFromContext(r.Context())
	logger := logr.FromContextOrDiscard(r.Context()).WithName("process-handler.process-get-sidecars")

	processGUID := chi.URLParam(r, "guid")

	_, err := h.processRepo.GetProcess(r.Context(), authInfo, processGUID)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, apierrors.ForbiddenAsNotFound(err), "Failed to fetch process from Kubernetes", "ProcessGUID", processGUID)
	}

	return routing.NewHandlerResponse(http.StatusOK).WithBody(map[string]interface{}{
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

func (h *ProcessHandler) processScaleHandler(r *http.Request) (*routing.Response, error) {
	authInfo, _ := authorization.InfoFromContext(r.Context())
	logger := logr.FromContextOrDiscard(r.Context()).WithName("process-handler.process-scale")

	processGUID := chi.URLParam(r, "guid")

	var payload payloads.ProcessScale
	if err := h.decoderValidator.DecodeAndValidateJSONPayload(r, &payload); err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "failed to decode payload")
	}

	processRecord, err := h.processScaler.ScaleProcess(r.Context(), authInfo, processGUID, payload.ToRecord())
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "Failed due to error from Kubernetes", "processGUID", processGUID)
	}

	return routing.NewHandlerResponse(http.StatusOK).WithBody(presenter.ForProcess(processRecord, h.serverURL)), nil
}

func (h *ProcessHandler) processGetStatsHandler(r *http.Request) (*routing.Response, error) {
	authInfo, _ := authorization.InfoFromContext(r.Context())
	logger := logr.FromContextOrDiscard(r.Context()).WithName("process-handler.process-get-stats")

	processGUID := chi.URLParam(r, "guid")

	records, err := h.processStatsFetcher.FetchStats(r.Context(), authInfo, processGUID)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, apierrors.ForbiddenAsNotFound(err), "Failed to get process stats from Kubernetes", "ProcessGUID", processGUID)
	}

	return routing.NewHandlerResponse(http.StatusOK).WithBody(presenter.ForProcessStats(records)), nil
}

func (h *ProcessHandler) processListHandler(r *http.Request) (*routing.Response, error) { //nolint:dupl
	authInfo, _ := authorization.InfoFromContext(r.Context())
	logger := logr.FromContextOrDiscard(r.Context()).WithName("process-handler.process-list")

	if err := r.ParseForm(); err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "Unable to parse request query parameters")
	}

	processListFilter := new(payloads.ProcessList)
	err := payloads.Decode(processListFilter, r.Form)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "Unable to decode request query parameters")
	}

	processList, err := h.processRepo.ListProcesses(r.Context(), authInfo, processListFilter.ToMessage())
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "Failed to fetch processes(s) from Kubernetes")
	}

	return routing.NewHandlerResponse(http.StatusOK).WithBody(presenter.ForProcessList(processList, h.serverURL, *r.URL)), nil
}

func (h *ProcessHandler) processPatchHandler(r *http.Request) (*routing.Response, error) {
	authInfo, _ := authorization.InfoFromContext(r.Context())
	logger := logr.FromContextOrDiscard(r.Context()).WithName("process-handler.process-patch")

	processGUID := chi.URLParam(r, "guid")

	var payload payloads.ProcessPatch
	if err := h.decoderValidator.DecodeAndValidateJSONPayload(r, &payload); err != nil {
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

	return routing.NewHandlerResponse(http.StatusOK).WithBody(presenter.ForProcess(updatedProcess, h.serverURL)), nil
}

func (h *ProcessHandler) RegisterRoutes(router *chi.Mux) {
	router.Method("GET", ProcessPath, routing.Handler(h.processGetHandler))
	router.Method("GET", ProcessSidecarsPath, routing.Handler(h.processGetSidecarsHandler))
	router.Method("POST", ProcessScalePath, routing.Handler(h.processScaleHandler))
	router.Method("GET", ProcessStatsPath, routing.Handler(h.processGetStatsHandler))
	router.Method("GET", ProcessesPath, routing.Handler(h.processListHandler))
	router.Method("PATCH", ProcessPath, routing.Handler(h.processPatchHandler))
}
