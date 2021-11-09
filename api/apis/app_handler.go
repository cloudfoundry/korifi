package apis

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"

	"code.cloudfoundry.org/cf-k8s-controllers/controllers/webhooks/workloads"

	"code.cloudfoundry.org/cf-k8s-controllers/api/payloads"
	"code.cloudfoundry.org/cf-k8s-controllers/api/presenter"
	"code.cloudfoundry.org/cf-k8s-controllers/api/repositories"

	"github.com/go-logr/logr"
	"github.com/gorilla/mux"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	AppCreateEndpoint            = "/v3/apps"
	AppGetEndpoint               = "/v3/apps/{guid}"
	AppListEndpoint              = "/v3/apps"
	AppSetCurrentDropletEndpoint = "/v3/apps/{guid}/relationships/current_droplet"
	AppGetCurrentDropletEndpoint = "/v3/apps/{guid}/droplets/current"
	AppGetProcessesEndpoint      = "/v3/apps/{guid}/processes"
	AppGetRoutesEndpoint         = "/v3/apps/{guid}/routes"
	AppStartEndpoint             = "/v3/apps/{guid}/actions/start"
	AppProcessScaleEndpoint      = "/v3/apps/{guid}/processes/{processType}/actions/scale"
	AppStopEndpoint              = "/v3/apps/{guid}/actions/stop"
	invalidDropletMsg            = "Unable to assign current droplet. Ensure the droplet exists and belongs to this app."

	AppStartedState = "STARTED"
	AppStoppedState = "STOPPED"
)

//counterfeiter:generate -o fake -fake-name CFAppRepository . CFAppRepository
type CFAppRepository interface {
	FetchApp(context.Context, client.Client, string) (repositories.AppRecord, error)
	FetchAppByNameAndSpace(context.Context, client.Client, string, string) (repositories.AppRecord, error)
	FetchAppList(context.Context, client.Client) ([]repositories.AppRecord, error)
	FetchNamespace(context.Context, client.Client, string) (repositories.SpaceRecord, error)
	CreateOrPatchAppEnvVars(context.Context, client.Client, repositories.CreateOrPatchAppEnvVarsMessage) (repositories.AppEnvVarsRecord, error)
	CreateApp(context.Context, client.Client, repositories.AppCreateMessage) (repositories.AppRecord, error)
	SetCurrentDroplet(context.Context, client.Client, repositories.SetCurrentDropletMessage) (repositories.CurrentDropletRecord, error)
	SetAppDesiredState(context.Context, client.Client, repositories.SetAppDesiredStateMessage) (repositories.AppRecord, error)
}

//counterfeiter:generate -o fake -fake-name ScaleAppProcess . ScaleAppProcess
type ScaleAppProcess func(ctx context.Context, client client.Client, appGUID string, processType string, scale repositories.ProcessScaleMessage) (repositories.ProcessRecord, error)

//counterfeiter:generate -o fake -fake-name CreateApp . CreateApp
type CreateApp func(context.Context, client.Client, payloads.AppCreate) (repositories.AppRecord, error)

type AppHandler struct {
	logger          logr.Logger
	serverURL       url.URL
	appRepo         CFAppRepository
	dropletRepo     CFDropletRepository
	processRepo     CFProcessRepository
	routeRepo       CFRouteRepository
	domainRepo      CFDomainRepository
	scaleAppProcess ScaleAppProcess
	createApp       CreateApp
	buildClient     ClientBuilder
	k8sConfig       *rest.Config // TODO: this would be global for all requests, not what we want
}

func NewAppHandler(
	logger logr.Logger,
	serverURL url.URL,
	appRepo CFAppRepository,
	dropletRepo CFDropletRepository,
	processRepo CFProcessRepository,
	routeRepo CFRouteRepository,
	domainRepo CFDomainRepository,
	scaleAppProcessFunc ScaleAppProcess,
	createAppFunc CreateApp,
	buildClient ClientBuilder,
	k8sConfig *rest.Config) *AppHandler {
	return &AppHandler{
		logger:          logger,
		serverURL:       serverURL,
		appRepo:         appRepo,
		dropletRepo:     dropletRepo,
		processRepo:     processRepo,
		routeRepo:       routeRepo,
		domainRepo:      domainRepo,
		scaleAppProcess: scaleAppProcessFunc,
		createApp:       createAppFunc,
		buildClient:     buildClient,
		k8sConfig:       k8sConfig,
	}
}

func (h *AppHandler) appGetHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	w.Header().Set("Content-Type", "application/json")

	vars := mux.Vars(r)
	appGUID := vars["guid"]

	// TODO: Instantiate config based on bearer token
	// Spike code from EMEA folks around this: https://github.com/cloudfoundry/cf-crd-explorations/blob/136417fbff507eb13c92cd67e6fed6b061071941/cfshim/handlers/app_handler.go#L78
	client, err := h.buildClient(h.k8sConfig)
	if err != nil {
		h.logger.Error(err, "Unable to create Kubernetes client", "AppGUID", appGUID)
		writeUnknownErrorResponse(w)
		return
	}

	app, err := h.appRepo.FetchApp(ctx, client, appGUID)
	if err != nil {
		switch err.(type) {
		case repositories.NotFoundError:
			h.logger.Info("App not found", "AppGUID", appGUID)
			writeNotFoundErrorResponse(w, "App")
			return
		default:
			h.logger.Error(err, "Failed to fetch app from Kubernetes", "AppGUID", appGUID)
			writeUnknownErrorResponse(w)
			return
		}
	}

	responseBody, err := json.Marshal(presenter.ForApp(app, h.serverURL))
	if err != nil {
		h.logger.Error(err, "Failed to render response", "AppGUID", appGUID)
		writeUnknownErrorResponse(w)
		return
	}

	w.Write(responseBody)
}

func (h *AppHandler) appCreateHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	w.Header().Set("Content-Type", "application/json")

	var payload payloads.AppCreate
	rme := decodeAndValidateJSONPayload(r, &payload)
	if rme != nil {
		writeErrorResponse(w, rme)
		return
	}

	// TODO: Instantiate config based on bearer token
	// Spike code from EMEA folks around this: https://github.com/cloudfoundry/cf-crd-explorations/blob/136417fbff507eb13c92cd67e6fed6b061071941/cfshim/handlers/app_handler.go#L78
	client, err := h.buildClient(h.k8sConfig)
	if err != nil {
		h.logger.Error(err, "Unable to create Kubernetes client")
		writeUnknownErrorResponse(w)
		return
	}

	// TODO: Move this into the action or its own "filter"
	namespaceGUID := payload.Relationships.Space.Data.GUID
	_, err = h.appRepo.FetchNamespace(ctx, client, namespaceGUID)

	if err != nil {
		switch err.(type) {
		case repositories.PermissionDeniedOrNotFoundError:
			h.logger.Info("Namespace not found", "Namespace GUID", namespaceGUID)
			writeUnprocessableEntityError(w, "Invalid space. Ensure that the space exists and you have access to it.")
			return
		default:
			h.logger.Error(err, "Failed to fetch namespace from Kubernetes", "Namespace GUID", namespaceGUID)
			writeUnknownErrorResponse(w)
			return
		}
	}

	appRecord, err := h.createApp(ctx, client, payload)
	if err != nil {
		if workloads.HasErrorCode(err, workloads.DuplicateAppError) {
			errorDetail := fmt.Sprintf("App with the name '%s' already exists.", payload.Name)
			h.logger.Error(err, errorDetail, "App Name", payload.Name)
			writeUniquenessError(w, errorDetail)
			return
		}
		h.logger.Error(err, "Failed to create app", "App Name", payload.Name)
		writeUnknownErrorResponse(w)
		return
	}

	responseBody, err := json.Marshal(presenter.ForApp(appRecord, h.serverURL))
	if err != nil {
		h.logger.Error(err, "Failed to render response", "App Name", payload.Name)
		writeUnknownErrorResponse(w)
		return
	}

	w.WriteHeader(http.StatusCreated)
	w.Write(responseBody)
}

func (h *AppHandler) appListHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	w.Header().Set("Content-Type", "application/json")

	// TODO: Instantiate config based on bearer token
	// Spike code from EMEA folks around this: https://github.com/cloudfoundry/cf-crd-explorations/blob/136417fbff507eb13c92cd67e6fed6b061071941/cfshim/handlers/app_handler.go#L78
	client, err := h.buildClient(h.k8sConfig)
	if err != nil {
		h.logger.Error(err, "Unable to create Kubernetes client")
		writeUnknownErrorResponse(w)
		return
	}

	appList, err := h.appRepo.FetchAppList(ctx, client)
	if err != nil {
		h.logger.Error(err, "Failed to fetch app(s) from Kubernetes")
		writeUnknownErrorResponse(w)
		return
	}

	responseBody, err := json.Marshal(presenter.ForAppList(appList, h.serverURL))
	if err != nil {
		h.logger.Error(err, "Failed to render response")
		writeUnknownErrorResponse(w)
		return
	}

	w.Write(responseBody)
}

func (h *AppHandler) appSetCurrentDropletHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	w.Header().Set("Content-Type", "application/json")
	vars := mux.Vars(r)
	appGUID := vars["guid"]

	var payload payloads.AppSetCurrentDroplet
	rme := decodeAndValidateJSONPayload(r, &payload)
	if rme != nil {
		writeErrorResponse(w, rme)
		return
	}

	// TODO: Instantiate config based on bearer token
	// Spike code from EMEA folks around this: https://github.com/cloudfoundry/cf-crd-explorations/blob/136417fbff507eb13c92cd67e6fed6b061071941/cfshim/handlers/app_handler.go#L78
	client, err := h.buildClient(h.k8sConfig)
	if err != nil {
		h.logger.Error(err, "Unable to create Kubernetes client")
		writeUnknownErrorResponse(w)
		return
	}

	app, err := h.appRepo.FetchApp(ctx, client, appGUID)
	if err != nil {
		if errors.As(err, new(repositories.NotFoundError)) {
			h.logger.Error(err, "App not found", "appGUID", app.GUID)
			writeNotFoundErrorResponse(w, "App")
		} else {
			h.logger.Error(err, "Error fetching app", "appGUID", app.GUID)
			writeUnknownErrorResponse(w)
		}
		return
	}

	dropletGUID := payload.Data.GUID
	droplet, err := h.dropletRepo.FetchDroplet(ctx, client, dropletGUID)
	if err != nil {
		if errors.As(err, new(repositories.NotFoundError)) {
			writeUnprocessableEntityError(w, invalidDropletMsg)
		} else {
			h.logger.Error(err, "Error fetching droplet")
			writeUnknownErrorResponse(w)
		}
		return
	}

	if droplet.AppGUID != appGUID {
		writeUnprocessableEntityError(w, invalidDropletMsg)
		return
	}

	currentDroplet, err := h.appRepo.SetCurrentDroplet(ctx, client, repositories.SetCurrentDropletMessage{
		AppGUID:     appGUID,
		DropletGUID: dropletGUID,
		SpaceGUID:   app.SpaceGUID,
	})
	if err != nil {
		h.logger.Error(err, "Error setting current droplet")
		writeUnknownErrorResponse(w)
		return
	}

	responseBody, err := json.Marshal(presenter.ForCurrentDroplet(currentDroplet, h.serverURL))
	if err != nil { // untested
		h.logger.Error(err, "Failed to render response")
		writeUnknownErrorResponse(w)
		return
	}

	w.Write(responseBody)
}

func (h *AppHandler) appGetCurrentDropletHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	w.Header().Set("Content-Type", "application/json")
	vars := mux.Vars(r)
	appGUID := vars["guid"]

	// TODO: Instantiate config based on bearer token
	client, err := h.buildClient(h.k8sConfig)
	if err != nil {
		h.logger.Error(err, "Unable to create Kubernetes client")
		writeUnknownErrorResponse(w)
		return
	}

	app, err := h.appRepo.FetchApp(ctx, client, appGUID)
	if err != nil {
		if errors.As(err, new(repositories.NotFoundError)) {
			h.logger.Error(err, "App not found", "appGUID", app.GUID)
			writeNotFoundErrorResponse(w, "App")
		} else {
			h.logger.Error(err, "Error fetching app", "appGUID", app.GUID)
			writeUnknownErrorResponse(w)
		}
		return
	}

	if app.DropletGUID == "" {
		h.logger.Info("App does not have a current droplet assigned", "appGUID", app.GUID)
		writeNotFoundErrorResponse(w, "Droplet")
		return
	}

	droplet, err := h.dropletRepo.FetchDroplet(ctx, client, app.DropletGUID)
	if err != nil {
		switch err.(type) {
		case repositories.NotFoundError:
			h.logger.Info("Droplet not found", "dropletGUID", app.DropletGUID)
			writeNotFoundErrorResponse(w, "Droplet")
			return
		default:
			h.logger.Error(err, "Failed to fetch droplet from Kubernetes", "dropletGUID", app.DropletGUID)
			writeUnknownErrorResponse(w)
			return
		}
	}

	responseBody, err := json.Marshal(presenter.ForDroplet(droplet, h.serverURL))
	if err != nil {
		h.logger.Error(err, "Failed to render response", "dropletGUID", app.DropletGUID)
		writeUnknownErrorResponse(w)
		return
	}
	w.Write(responseBody)
}

func (h *AppHandler) appStartHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	w.Header().Set("Content-Type", "application/json")

	vars := mux.Vars(r)
	appGUID := vars["guid"]

	// TODO: Instantiate config based on bearer token
	// Spike code from EMEA folks around this: https://github.com/cloudfoundry/cf-crd-explorations/blob/136417fbff507eb13c92cd67e6fed6b061071941/cfshim/handlers/app_handler.go#L78
	client, err := h.buildClient(h.k8sConfig)
	if err != nil {
		h.logger.Error(err, "Unable to create Kubernetes client", "AppGUID", appGUID)
		writeUnknownErrorResponse(w)
		return
	}

	app, err := h.appRepo.FetchApp(ctx, client, appGUID)
	if err != nil {
		switch err.(type) {
		case repositories.NotFoundError:
			h.logger.Info("App not found", "AppGUID", appGUID)
			writeNotFoundErrorResponse(w, "App")
			return
		default:
			h.logger.Error(err, "Failed to fetch app from Kubernetes", "AppGUID", appGUID)
			writeUnknownErrorResponse(w)
			return
		}
	}
	if app.DropletGUID == "" {
		h.logger.Info("App droplet not set before start", "AppGUID", appGUID)
		writeUnprocessableEntityError(w, "Assign a droplet before starting this app.")
		return
	}

	app, err = h.appRepo.SetAppDesiredState(ctx, client, repositories.SetAppDesiredStateMessage{
		AppGUID:      app.GUID,
		SpaceGUID:    app.SpaceGUID,
		DesiredState: AppStartedState,
	})
	if err != nil {
		h.logger.Error(err, "Failed to update app in Kubernetes", "AppGUID", appGUID)
		writeUnknownErrorResponse(w)
		return
	}

	responseBody, err := json.Marshal(presenter.ForApp(app, h.serverURL))
	if err != nil {
		h.logger.Error(err, "Failed to render response", "AppGUID", appGUID)
		writeUnknownErrorResponse(w)
		return
	}

	w.Write(responseBody)
}

func (h *AppHandler) appStopHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	w.Header().Set("Content-Type", "application/json")

	vars := mux.Vars(r)
	appGUID := vars["guid"]

	// TODO: Instantiate config based on bearer token
	// Spike code from EMEA folks around this: https://github.com/cloudfoundry/cf-crd-explorations/blob/136417fbff507eb13c92cd67e6fed6b061071941/cfshim/handlers/app_handler.go#L78
	client, err := h.buildClient(h.k8sConfig)
	if err != nil {
		h.logger.Error(err, "Unable to create Kubernetes client", "AppGUID", appGUID)
		writeUnknownErrorResponse(w)
		return
	}

	app, err := h.appRepo.FetchApp(ctx, client, appGUID)
	if err != nil {
		switch err.(type) {
		case repositories.NotFoundError:
			h.logger.Info("App not found", "AppGUID", appGUID)
			writeNotFoundErrorResponse(w, "App")
			return
		default:
			h.logger.Error(err, "Failed to fetch app from Kubernetes", "AppGUID", appGUID)
			writeUnknownErrorResponse(w)
			return
		}
	}

	app, err = h.appRepo.SetAppDesiredState(ctx, client, repositories.SetAppDesiredStateMessage{
		AppGUID:      app.GUID,
		SpaceGUID:    app.SpaceGUID,
		DesiredState: AppStoppedState,
	})
	if err != nil {
		h.logger.Error(err, "Failed to update app in Kubernetes", "AppGUID", appGUID)
		writeUnknownErrorResponse(w)
		return
	}

	responseBody, err := json.Marshal(presenter.ForApp(app, h.serverURL))
	if err != nil {
		h.logger.Error(err, "Failed to render response", "AppGUID", appGUID)
		writeUnknownErrorResponse(w)
		return
	}

	w.Write(responseBody)
}

func (h *AppHandler) getProcessesForAppHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	w.Header().Set("Content-Type", "application/json")

	vars := mux.Vars(r)
	appGUID := vars["guid"]

	// TODO: Instantiate config based on bearer token
	// Spike code from EMEA folks around this: https://github.com/cloudfoundry/cf-crd-explorations/blob/136417fbff507eb13c92cd67e6fed6b061071941/cfshim/handlers/app_handler.go#L78
	client, err := h.buildClient(h.k8sConfig)
	if err != nil {
		h.logger.Error(err, "Unable to create Kubernetes client", "AppGUID", appGUID)
		writeUnknownErrorResponse(w)
		return
	}

	app, err := h.appRepo.FetchApp(ctx, client, appGUID)
	if err != nil {
		switch err.(type) {
		case repositories.NotFoundError:
			h.logger.Info("App not found", "AppGUID", appGUID)
			writeNotFoundErrorResponse(w, "App")
			return
		default:
			h.logger.Error(err, "Failed to fetch app from Kubernetes", "AppGUID", appGUID)
			writeUnknownErrorResponse(w)
			return
		}
	}

	processList, err := h.processRepo.FetchProcessesForApp(ctx, client, appGUID, app.SpaceGUID)
	if err != nil {
		h.logger.Error(err, "Failed to fetch app Process(es) from Kubernetes")
		writeUnknownErrorResponse(w)
		return
	}

	responseBody, err := json.Marshal(presenter.ForProcessList(processList, h.serverURL, appGUID))
	if err != nil { // untested
		h.logger.Error(err, "Failed to render response")
		writeUnknownErrorResponse(w)
		return
	}

	w.Write(responseBody)
}

func (h *AppHandler) getRoutesForAppHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	w.Header().Set("Content-Type", "application/json")

	vars := mux.Vars(r)
	appGUID := vars["guid"]

	// TODO: Instantiate config based on bearer token
	// Spike code from EMEA folks around this: https://github.com/cloudfoundry/cf-crd-explorations/blob/136417fbff507eb13c92cd67e6fed6b061071941/cfshim/handlers/app_handler.go#L78
	client, err := h.buildClient(h.k8sConfig)
	if err != nil {
		h.logger.Error(err, "Unable to create Kubernetes client", "AppGUID", appGUID)
		writeUnknownErrorResponse(w)
		return
	}

	app, err := h.appRepo.FetchApp(ctx, client, appGUID)
	if err != nil {
		switch err.(type) {
		case repositories.NotFoundError:
			h.logger.Info("App not found", "AppGUID", appGUID)
			writeNotFoundErrorResponse(w, "App")
			return
		default:
			h.logger.Error(err, "Failed to fetch app from Kubernetes", "AppGUID", appGUID)
			writeUnknownErrorResponse(w)
			return
		}
	}

	routes, err := h.lookupAppRouteAndDomainList(ctx, client, app.GUID, app.SpaceGUID)
	if err != nil {
		h.logger.Error(err, "Failed to fetch route or domains from Kubernetes")
		writeUnknownErrorResponse(w)
		return
	}

	responseBody, err := json.Marshal(presenter.ForAppRouteList(routes, h.serverURL, app.GUID))
	if err != nil {
		h.logger.Error(err, "Failed to render response")
		writeUnknownErrorResponse(w)
		return
	}

	w.Write(responseBody)
}

func (h *AppHandler) appScaleProcessHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	w.Header().Set("Content-Type", "application/json")

	vars := mux.Vars(r)
	appGUID := vars["guid"]
	processType := vars["processType"]

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
		h.logger.Error(err, "Unable to create Kubernetes client", "AppGUID", appGUID)
		writeUnknownErrorResponse(w)
		return
	}

	processRecord, err := h.scaleAppProcess(ctx, client, appGUID, processType, payload.ToRecord())
	if err != nil {
		switch errType := err.(type) {
		case repositories.NotFoundError:
			resourceType := errType.ResourceType
			h.logger.Info(fmt.Sprintf("%s not found", resourceType), "appGUID", appGUID)
			writeNotFoundErrorResponse(w, resourceType)
			return
		default:
			h.logger.Error(err, "Failed due to error from Kubernetes", "appGUID", appGUID)
			writeUnknownErrorResponse(w)
			return
		}
	}

	responseBody, err := json.Marshal(presenter.ForProcess(processRecord, h.serverURL))
	if err != nil {
		h.logger.Error(err, "Failed to render response", "ProcessGUID", appGUID)
		writeUnknownErrorResponse(w)
		return
	}

	w.Write(responseBody)
}

func (h *AppHandler) lookupAppRouteAndDomainList(ctx context.Context, client client.Client, appGUID, spaceGUID string) ([]repositories.RouteRecord, error) {

	routeRecords, err := h.routeRepo.FetchRoutesForApp(ctx, client, appGUID, spaceGUID)
	if err != nil {
		return []repositories.RouteRecord{}, err
	}

	return getDomainsForRoutes(ctx, h.domainRepo, client, routeRecords)
}

func (h *AppHandler) RegisterRoutes(router *mux.Router) {
	router.Path(AppGetEndpoint).Methods("GET").HandlerFunc(h.appGetHandler)
	router.Path(AppListEndpoint).Methods("GET").HandlerFunc(h.appListHandler)
	router.Path(AppCreateEndpoint).Methods("POST").HandlerFunc(h.appCreateHandler)
	router.Path(AppSetCurrentDropletEndpoint).Methods("PATCH").HandlerFunc(h.appSetCurrentDropletHandler)
	router.Path(AppGetCurrentDropletEndpoint).Methods("GET").HandlerFunc(h.appGetCurrentDropletHandler)
	router.Path(AppStartEndpoint).Methods("POST").HandlerFunc(h.appStartHandler)
	router.Path(AppStopEndpoint).Methods("POST").HandlerFunc(h.appStopHandler)
	router.Path(AppProcessScaleEndpoint).Methods("POST").HandlerFunc(h.appScaleProcessHandler)
	router.Path(AppGetProcessesEndpoint).Methods("GET").HandlerFunc(h.getProcessesForAppHandler)
	router.Path(AppGetRoutesEndpoint).Methods("GET").HandlerFunc(h.getRoutesForAppHandler)
}
