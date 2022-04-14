package apis

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	"github.com/gorilla/schema"

	"code.cloudfoundry.org/korifi/api/apierrors"
	"code.cloudfoundry.org/korifi/api/authorization"
	"code.cloudfoundry.org/korifi/api/payloads"
	"code.cloudfoundry.org/korifi/api/presenter"
	"code.cloudfoundry.org/korifi/api/repositories"

	"github.com/go-logr/logr"
	"github.com/gorilla/mux"
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

//counterfeiter:generate -o fake -fake-name ScaleProcess . ScaleProcess
type ScaleProcess func(ctx context.Context, authInfo authorization.Info, processGUID string, scale repositories.ProcessScaleValues) (repositories.ProcessRecord, error)

//counterfeiter:generate -o fake -fake-name FetchProcessStats . FetchProcessStats
type FetchProcessStats func(context.Context, authorization.Info, string) ([]repositories.PodStatsRecord, error)

type ProcessHandler struct {
	logger            logr.Logger
	serverURL         url.URL
	processRepo       CFProcessRepository
	fetchProcessStats FetchProcessStats
	scaleProcess      ScaleProcess
	decoderValidator  *DecoderValidator
}

func NewProcessHandler(
	logger logr.Logger,
	serverURL url.URL,
	processRepo CFProcessRepository,
	fetchProcessStats FetchProcessStats,
	scaleProcessFunc ScaleProcess,
	decoderValidator *DecoderValidator,
) *ProcessHandler {
	return &ProcessHandler{
		logger:            logger,
		serverURL:         serverURL,
		processRepo:       processRepo,
		fetchProcessStats: fetchProcessStats,
		scaleProcess:      scaleProcessFunc,
		decoderValidator:  decoderValidator,
	}
}

func (h *ProcessHandler) processGetHandler(authInfo authorization.Info, r *http.Request) (*HandlerResponse, error) {
	ctx := r.Context()

	vars := mux.Vars(r)
	processGUID := vars["guid"]

	process, err := h.processRepo.GetProcess(ctx, authInfo, processGUID)
	if err != nil {
		h.logger.Error(err, "Failed to fetch process from Kubernetes", "ProcessGUID", processGUID)
		return nil, apierrors.ForbiddenAsNotFound(err)
	}

	return NewHandlerResponse(http.StatusOK).WithBody(presenter.ForProcess(process, h.serverURL)), nil
}

func (h *ProcessHandler) processGetSidecarsHandler(authInfo authorization.Info, r *http.Request) (*HandlerResponse, error) {
	ctx := r.Context()

	vars := mux.Vars(r)
	processGUID := vars["guid"]

	_, err := h.processRepo.GetProcess(ctx, authInfo, processGUID)
	if err != nil {
		h.logger.Error(err, "Failed to fetch process from Kubernetes", "ProcessGUID", processGUID)
		return nil, apierrors.ForbiddenAsNotFound(err)
	}

	return NewHandlerResponse(http.StatusOK).WithBody(map[string]interface{}{
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

func (h *ProcessHandler) processScaleHandler(authInfo authorization.Info, r *http.Request) (*HandlerResponse, error) {
	ctx := r.Context()

	vars := mux.Vars(r)
	processGUID := vars["guid"]

	var payload payloads.ProcessScale
	if err := h.decoderValidator.DecodeAndValidateJSONPayload(r, &payload); err != nil {
		return nil, err
	}

	processRecord, err := h.scaleProcess(ctx, authInfo, processGUID, payload.ToRecord())
	if err != nil {
		h.logger.Error(err, "Failed due to error from Kubernetes", "processGUID", processGUID)
		return nil, err
	}

	return NewHandlerResponse(http.StatusOK).WithBody(presenter.ForProcess(processRecord, h.serverURL)), nil
}

func (h *ProcessHandler) processGetStatsHandler(authInfo authorization.Info, r *http.Request) (*HandlerResponse, error) {
	ctx := r.Context()

	vars := mux.Vars(r)
	processGUID := vars["guid"]

	records, err := h.fetchProcessStats(ctx, authInfo, processGUID)
	if err != nil {
		h.logger.Error(err, "Failed to get process stats from Kubernetes", "ProcessGUID", processGUID)
		return nil, apierrors.ForbiddenAsNotFound(err)
	}

	return NewHandlerResponse(http.StatusOK).WithBody(presenter.ForProcessStats(records)), nil
}

func (h *ProcessHandler) processListHandler(authInfo authorization.Info, r *http.Request) (*HandlerResponse, error) { //nolint:dupl
	ctx := r.Context()

	if err := r.ParseForm(); err != nil {
		h.logger.Error(err, "Unable to parse request query parameters")
		return nil, err
	}

	processListFilter := new(payloads.ProcessList)
	err := schema.NewDecoder().Decode(processListFilter, r.Form)
	if err != nil {
		switch err.(type) {
		case schema.MultiError:
			multiError := err.(schema.MultiError)
			for _, v := range multiError {
				_, ok := v.(schema.UnknownKeyError)
				if ok {
					h.logger.Info("Unknown key used in Process filter")
					return nil, apierrors.NewUnknownKeyError(err, processListFilter.SupportedFilterKeys())
				}
			}
			h.logger.Error(err, "Unable to decode request query parameters")
			return nil, err

		default:
			h.logger.Error(err, "Unable to decode request query parameters")
			return nil, err
		}
	}

	processList, err := h.processRepo.ListProcesses(ctx, authInfo, processListFilter.ToMessage())
	if err != nil {
		h.logger.Error(err, "Failed to fetch processes(s) from Kubernetes")
		return nil, err
	}

	return NewHandlerResponse(http.StatusOK).WithBody(presenter.ForProcessList(processList, h.serverURL, *r.URL)), nil
}

func (h *ProcessHandler) processPatchHandler(authInfo authorization.Info, r *http.Request) (*HandlerResponse, error) {
	ctx := r.Context()

	vars := mux.Vars(r)
	processGUID := vars["guid"]

	var payload payloads.ProcessPatch
	if err := h.decoderValidator.DecodeAndValidateJSONPayload(r, &payload); err != nil {
		return nil, err
	}

	process, err := h.processRepo.GetProcess(ctx, authInfo, processGUID)
	if err != nil {
		h.logger.Error(err, "Failed to get process from Kubernetes", "ProcessGUID", processGUID)
		return nil, apierrors.ForbiddenAsNotFound(err)
	}

	updatedProcess, err := h.processRepo.PatchProcess(ctx, authInfo, payload.ToProcessPatchMessage(processGUID, process.SpaceGUID))
	if err != nil {
		h.logger.Error(err, "Failed to patch process from Kubernetes", "ProcessGUID", processGUID)
		return nil, err
	}

	return NewHandlerResponse(http.StatusOK).WithBody(presenter.ForProcess(updatedProcess, h.serverURL)), nil
}

func (h *ProcessHandler) RegisterRoutes(router *mux.Router) {
	w := NewAuthAwareHandlerFuncWrapper(h.logger)
	router.Path(ProcessPath).Methods("GET").HandlerFunc(w.Wrap(h.processGetHandler))
	router.Path(ProcessSidecarsPath).Methods("GET").HandlerFunc(w.Wrap(h.processGetSidecarsHandler))
	router.Path(ProcessScalePath).Methods("POST").HandlerFunc(w.Wrap(h.processScaleHandler))
	router.Path(ProcessStatsPath).Methods("GET").HandlerFunc(w.Wrap(h.processGetStatsHandler))
	router.Path(ProcessesPath).Methods("GET").HandlerFunc(w.Wrap(h.processListHandler))
	router.Path(ProcessPath).Methods("PATCH").HandlerFunc(w.Wrap(h.processPatchHandler))
}
