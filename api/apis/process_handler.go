package apis

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"github.com/gorilla/schema"

	"code.cloudfoundry.org/cf-k8s-controllers/api/authorization"
	"code.cloudfoundry.org/cf-k8s-controllers/api/payloads"
	"code.cloudfoundry.org/cf-k8s-controllers/api/presenter"
	"code.cloudfoundry.org/cf-k8s-controllers/api/repositories"

	"github.com/go-logr/logr"
	"github.com/gorilla/mux"
)

const (
	ProcessGetEndpoint         = "/v3/processes/{guid}"
	ProcessGetSidecarsEndpoint = "/v3/processes/{guid}/sidecars"
	ProcessScaleEndpoint       = "/v3/processes/{guid}/actions/scale"
	ProcessGetStatsEndpoint    = "/v3/processes/{guid}/stats"
	ProcessListEndpoint        = "/v3/processes"
)

//counterfeiter:generate -o fake -fake-name CFProcessRepository . CFProcessRepository
type CFProcessRepository interface {
	FetchProcess(context.Context, authorization.Info, string) (repositories.ProcessRecord, error)
	FetchProcessList(context.Context, authorization.Info, repositories.FetchProcessListMessage) ([]repositories.ProcessRecord, error)
}

//counterfeiter:generate -o fake -fake-name PodRepository . PodRepository
type PodRepository interface {
	FetchPodStatsByAppGUID(context.Context, authorization.Info, repositories.FetchPodStatsMessage) ([]repositories.PodStatsRecord, error)
	WatchForPodsTermination(context.Context, authorization.Info, string, string) (bool, error)
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
}

func NewProcessHandler(
	logger logr.Logger,
	serverURL url.URL,
	processRepo CFProcessRepository,
	fetchProcessStats FetchProcessStats,
	scaleProcessFunc ScaleProcess,
) *ProcessHandler {
	return &ProcessHandler{
		logger:            logger,
		serverURL:         serverURL,
		processRepo:       processRepo,
		fetchProcessStats: fetchProcessStats,
		scaleProcess:      scaleProcessFunc,
	}
}

func (h *ProcessHandler) processGetHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	w.Header().Set("Content-Type", "application/json")

	vars := mux.Vars(r)
	processGUID := vars["guid"]

	authInfo, ok := authorization.InfoFromContext(r.Context())
	if !ok {
		h.logger.Error(nil, "unable to get auth info")
		writeUnknownErrorResponse(w)
		return
	}

	process, err := h.processRepo.FetchProcess(ctx, authInfo, processGUID)
	if err != nil {
		h.LogError(w, processGUID, err)
		return
	}

	responseBody, err := json.Marshal(presenter.ForProcess(process, h.serverURL))
	if err != nil {
		h.logger.Error(err, "Failed to render response", "ProcessGUID", processGUID)
		writeUnknownErrorResponse(w)
		return
	}

	_, _ = w.Write(responseBody)
}

func (h *ProcessHandler) processGetSidecarsHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	w.Header().Set("Content-Type", "application/json")

	vars := mux.Vars(r)
	processGUID := vars["guid"]

	authInfo, ok := authorization.InfoFromContext(r.Context())
	if !ok {
		h.logger.Error(nil, "unable to get auth info")
		writeUnknownErrorResponse(w)
		return
	}

	_, err := h.processRepo.FetchProcess(ctx, authInfo, processGUID)
	if err != nil {
		h.LogError(w, processGUID, err)
		return
	}

	_, _ = w.Write([]byte(fmt.Sprintf(`{
					"pagination": {
						"total_results": 0,
						"total_pages": 1,
						"first": {
							"href": "%[1]s/v3/processes/%[2]s/sidecars?page=1"
						},
						"last": {
							"href": "%[1]s/v3/processes/%[2]s/sidecars?page=1"
						},
						"next": null,
						"previous": null
					},
					"resources": []
				}`, h.serverURL.String(), processGUID)))
}

func (h *ProcessHandler) processScaleHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	w.Header().Set("Content-Type", "application/json")

	vars := mux.Vars(r)
	processGUID := vars["guid"]

	var payload payloads.ProcessScale
	rme := decodeAndValidateJSONPayload(r, &payload)
	if rme != nil {
		writeRequestMalformedErrorResponse(w, rme)
		return
	}

	authInfo, ok := authorization.InfoFromContext(r.Context())
	if !ok {
		h.logger.Error(nil, "unable to get auth info")
		writeUnknownErrorResponse(w)
		return
	}

	processRecord, err := h.scaleProcess(ctx, authInfo, processGUID, payload.ToRecord())
	if err != nil {
		switch err.(type) {
		case repositories.NotFoundError:
			h.logger.Info("Process not found", "processGUID", processGUID)
			writeNotFoundErrorResponse(w, "Process")
			return
		default:
			h.logger.Error(err, "Failed due to error from Kubernetes", "processGUID", processGUID)
			writeUnknownErrorResponse(w)
			return
		}
	}

	responseBody, err := json.Marshal(presenter.ForProcess(processRecord, h.serverURL))
	if err != nil {
		h.logger.Error(err, "Failed to render response", "ProcessGUID", processGUID)
		writeUnknownErrorResponse(w)
		return
	}

	_, _ = w.Write(responseBody)
}

func (h *ProcessHandler) processGetStatsHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	w.Header().Set("Content-Type", "application/json")

	vars := mux.Vars(r)
	processGUID := vars["guid"]

	authInfo, ok := authorization.InfoFromContext(r.Context())
	if !ok {
		h.logger.Error(nil, "unable to get auth info")
		writeUnknownErrorResponse(w)
		return
	}

	records, err := h.fetchProcessStats(ctx, authInfo, processGUID)
	if err != nil {
		h.LogError(w, processGUID, err)
		return
	}

	responseBody, err := json.Marshal(presenter.ForProcessStats(records))
	if err != nil {
		h.LogError(w, processGUID, err)
		return
	}

	_, _ = w.Write(responseBody)
}

func (h *ProcessHandler) processListHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	w.Header().Set("Content-Type", "application/json")

	if err := r.ParseForm(); err != nil {
		h.logger.Error(err, "Unable to parse request query parameters")
		writeUnknownErrorResponse(w)
		return
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
					writeUnknownKeyError(w, processListFilter.SupportedFilterKeys())
					return
				}
			}
			h.logger.Error(err, "Unable to decode request query parameters")
			writeUnknownErrorResponse(w)
			return

		default:
			h.logger.Error(err, "Unable to decode request query parameters")
			writeUnknownErrorResponse(w)
			return
		}
	}

	authInfo, ok := authorization.InfoFromContext(r.Context())
	if !ok {
		h.logger.Error(nil, "unable to get auth info")
		writeUnknownErrorResponse(w)
		return
	}

	processList, err := h.processRepo.FetchProcessList(ctx, authInfo, processListFilter.ToMessage())
	if err != nil {
		h.logger.Error(err, "Failed to fetch processes(s) from Kubernetes")
		writeUnknownErrorResponse(w)
		return
	}

	responseBody, err := json.Marshal(presenter.ForProcessList(processList, h.serverURL, *processListFilter))
	if err != nil {
		h.logger.Error(err, "Failed to render response")
		writeUnknownErrorResponse(w)
		return
	}

	_, _ = w.Write(responseBody)
}

func (h *ProcessHandler) LogError(w http.ResponseWriter, processGUID string, err error) {
	switch tycerr := err.(type) {
	case repositories.NotFoundError:
		h.logger.Info(fmt.Sprintf("%s not found", tycerr.ResourceType), "ProcessGUID", processGUID)
		writeNotFoundErrorResponse(w, tycerr.ResourceType)
	default:
		h.logger.Error(err, "Failed to fetch process from Kubernetes", "ProcessGUID", processGUID)
		writeUnknownErrorResponse(w)
	}
}

func (h *ProcessHandler) RegisterRoutes(router *mux.Router) {
	router.Path(ProcessGetEndpoint).Methods("GET").HandlerFunc(h.processGetHandler)
	router.Path(ProcessGetSidecarsEndpoint).Methods("GET").HandlerFunc(h.processGetSidecarsHandler)
	router.Path(ProcessScaleEndpoint).Methods("POST").HandlerFunc(h.processScaleHandler)
	router.Path(ProcessGetStatsEndpoint).Methods("GET").HandlerFunc(h.processGetStatsHandler)
	router.Path(ProcessListEndpoint).Methods("GET").HandlerFunc(h.processListHandler)
}
