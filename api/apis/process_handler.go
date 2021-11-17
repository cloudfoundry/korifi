package apis

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"code.cloudfoundry.org/cf-k8s-controllers/api/payloads"
	"code.cloudfoundry.org/cf-k8s-controllers/api/presenter"
	"code.cloudfoundry.org/cf-k8s-controllers/api/repositories"

	"github.com/go-logr/logr"
	"github.com/gorilla/mux"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	ProcessGetEndpoint         = "/v3/processes/{guid}"
	ProcessGetSidecarsEndpoint = "/v3/processes/{guid}/sidecars"
	ProcessScaleEndpoint       = "/v3/processes/{guid}/actions/scale"
	ProcessGetStatsEndpoint    = "/v3/processes/{guid}/stats"
)

//counterfeiter:generate -o fake -fake-name CFProcessRepository . CFProcessRepository
type CFProcessRepository interface {
	FetchProcess(context.Context, client.Client, string) (repositories.ProcessRecord, error)
	FetchProcessesForApp(context.Context, client.Client, string, string) ([]repositories.ProcessRecord, error)
}

//counterfeiter:generate -o fake -fake-name PodRepository . PodRepository
type PodRepository interface {
	FetchPodStatsByAppGUID(context.Context, client.Client, repositories.FetchPodStatsMessage) ([]repositories.PodStatsRecord, error)
	WatchForPodsTermination(context.Context, client.Client, string, string) (bool, error)
}

//counterfeiter:generate -o fake -fake-name ScaleProcess . ScaleProcess
type ScaleProcess func(ctx context.Context, client client.Client, processGUID string, scale repositories.ProcessScaleValues) (repositories.ProcessRecord, error)

//counterfeiter:generate -o fake -fake-name FetchProcessStats . FetchProcessStats
type FetchProcessStats func(context.Context, client.Client, string) ([]repositories.PodStatsRecord, error)

type ProcessHandler struct {
	logger            logr.Logger
	serverURL         url.URL
	processRepo       CFProcessRepository
	fetchProcessStats FetchProcessStats
	scaleProcess      ScaleProcess
	buildClient       ClientBuilder
	k8sConfig         *rest.Config // TODO: this would be global for all requests, not what we want
}

func NewProcessHandler(
	logger logr.Logger,
	serverURL url.URL,
	processRepo CFProcessRepository,
	fetchProcessStats FetchProcessStats,
	scaleProcessFunc ScaleProcess,
	buildClient ClientBuilder,
	k8sConfig *rest.Config) *ProcessHandler {
	return &ProcessHandler{
		logger:            logger,
		serverURL:         serverURL,
		processRepo:       processRepo,
		fetchProcessStats: fetchProcessStats,
		scaleProcess:      scaleProcessFunc,
		buildClient:       buildClient,
		k8sConfig:         k8sConfig,
	}
}

func (h *ProcessHandler) processGetHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	w.Header().Set("Content-Type", "application/json")

	vars := mux.Vars(r)
	processGUID := vars["guid"]

	client, err := h.buildClient(h.k8sConfig)
	if err != nil {
		h.logger.Error(err, "Unable to create Kubernetes client", "ProcessGUID", processGUID)
		writeUnknownErrorResponse(w)
		return
	}

	process, err := h.processRepo.FetchProcess(ctx, client, processGUID)
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

	w.Write(responseBody)
}

func (h *ProcessHandler) processGetSidecarsHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	w.Header().Set("Content-Type", "application/json")

	vars := mux.Vars(r)
	processGUID := vars["guid"]

	// TODO: Instantiate config based on bearer token
	// Spike code from EMEA folks around this: https://github.com/cloudfoundry/cf-crd-explorations/blob/136417fbff507eb13c92cd67e6fed6b061071941/cfshim/handlers/app_handler.go#L78
	client, err := h.buildClient(h.k8sConfig)
	if err != nil {
		h.logger.Error(err, "Unable to create Kubernetes client", "ProcessGUID", processGUID)
		writeUnknownErrorResponse(w)
		return
	}

	_, err = h.processRepo.FetchProcess(ctx, client, processGUID)
	if err != nil {
		h.LogError(w, processGUID, err)
		return
	}

	w.Write([]byte(fmt.Sprintf(`{
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
		writeErrorResponse(w, rme)
		return
	}

	// TODO: Instantiate config based on bearer token
	// Spike code from EMEA folks around this: https://github.com/cloudfoundry/cf-crd-explorations/blob/136417fbff507eb13c92cd67e6fed6b061071941/cfshim/handlers/app_handler.go#L78
	client, err := h.buildClient(h.k8sConfig)
	if err != nil {
		h.logger.Error(err, "Unable to create Kubernetes client", "ProcessGUID", processGUID)
		writeUnknownErrorResponse(w)
		return
	}

	processRecord, err := h.scaleProcess(ctx, client, processGUID, payload.ToRecord())
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

	w.Write(responseBody)
}

func (h *ProcessHandler) processGetStatsHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	w.Header().Set("Content-Type", "application/json")

	vars := mux.Vars(r)
	processGUID := vars["guid"]

	// TODO: Instantiate config based on bearer token
	// Spike code from EMEA folks around this: https://github.com/cloudfoundry/cf-crd-explorations/blob/136417fbff507eb13c92cd67e6fed6b061071941/cfshim/handlers/app_handler.go#L78
	client, err := h.buildClient(h.k8sConfig)
	if err != nil {
		h.logger.Error(err, "Unable to create Kubernetes client", "ProcessGUID", processGUID)
		writeUnknownErrorResponse(w)
		return
	}

	records, err := h.fetchProcessStats(ctx, client, processGUID)
	if err != nil {
		h.LogError(w, processGUID, err)
		return
	}

	responseBody, err := json.Marshal(presenter.ForProcessStats(records))
	if err != nil {
		h.LogError(w, processGUID, err)
		return
	}

	w.Write(responseBody)
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
}
