package apis

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"

	"code.cloudfoundry.org/cf-k8s-controllers/controllers/webhooks/workloads"
	"github.com/gorilla/schema"

	"code.cloudfoundry.org/cf-k8s-controllers/api/authorization"
	"code.cloudfoundry.org/cf-k8s-controllers/api/payloads"
	"code.cloudfoundry.org/cf-k8s-controllers/api/presenter"
	"code.cloudfoundry.org/cf-k8s-controllers/api/repositories"

	"github.com/go-logr/logr"
	"github.com/gorilla/mux"
)

const (
	AppCreateEndpoint            = "/v3/apps"
	AppGetEndpoint               = "/v3/apps/{guid}"
	AppListEndpoint              = "/v3/apps"
	AppSetCurrentDropletEndpoint = "/v3/apps/{guid}/relationships/current_droplet"
	AppGetCurrentDropletEndpoint = "/v3/apps/{guid}/droplets/current"
	AppGetProcessesEndpoint      = "/v3/apps/{guid}/processes"
	AppProcessScaleEndpoint      = "/v3/apps/{guid}/processes/{processType}/actions/scale"
	AppGetRoutesEndpoint         = "/v3/apps/{guid}/routes"
	AppStartEndpoint             = "/v3/apps/{guid}/actions/start"
	AppStopEndpoint              = "/v3/apps/{guid}/actions/stop"
	AppRestartEndpoint           = "/v3/apps/{guid}/actions/restart"
	invalidDropletMsg            = "Unable to assign current droplet. Ensure the droplet exists and belongs to this app."

	AppStartedState = "STARTED"
	AppStoppedState = "STOPPED"
)

//counterfeiter:generate -o fake -fake-name CFAppRepository . CFAppRepository
type CFAppRepository interface {
	FetchApp(context.Context, authorization.Info, string) (repositories.AppRecord, error)
	FetchAppByNameAndSpace(context.Context, authorization.Info, string, string) (repositories.AppRecord, error)
	FetchAppList(context.Context, authorization.Info, repositories.AppListMessage) ([]repositories.AppRecord, error)
	FetchNamespace(context.Context, authorization.Info, string) (repositories.SpaceRecord, error)
	CreateOrPatchAppEnvVars(context.Context, authorization.Info, repositories.CreateOrPatchAppEnvVarsMessage) (repositories.AppEnvVarsRecord, error)
	CreateApp(context.Context, authorization.Info, repositories.AppCreateMessage) (repositories.AppRecord, error)
	SetCurrentDroplet(context.Context, authorization.Info, repositories.SetCurrentDropletMessage) (repositories.CurrentDropletRecord, error)
	SetAppDesiredState(context.Context, authorization.Info, repositories.SetAppDesiredStateMessage) (repositories.AppRecord, error)
}

//counterfeiter:generate -o fake -fake-name ScaleAppProcess . ScaleAppProcess
type ScaleAppProcess func(ctx context.Context, authInfo authorization.Info, appGUID string, processType string, scale repositories.ProcessScaleValues) (repositories.ProcessRecord, error)

type AppHandler struct {
	logger          logr.Logger
	serverURL       url.URL
	appRepo         CFAppRepository
	dropletRepo     CFDropletRepository
	processRepo     CFProcessRepository
	routeRepo       CFRouteRepository
	domainRepo      CFDomainRepository
	podRepo         PodRepository
	scaleAppProcess ScaleAppProcess
}

func NewAppHandler(
	logger logr.Logger,
	serverURL url.URL,
	appRepo CFAppRepository,
	dropletRepo CFDropletRepository,
	processRepo CFProcessRepository,
	routeRepo CFRouteRepository,
	domainRepo CFDomainRepository,
	podRepo PodRepository,
	scaleAppProcessFunc ScaleAppProcess,
) *AppHandler {
	return &AppHandler{
		logger:          logger,
		serverURL:       serverURL,
		appRepo:         appRepo,
		dropletRepo:     dropletRepo,
		processRepo:     processRepo,
		routeRepo:       routeRepo,
		domainRepo:      domainRepo,
		podRepo:         podRepo,
		scaleAppProcess: scaleAppProcessFunc,
	}
}

func (h *AppHandler) appGetHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	w.Header().Set("Content-Type", "application/json")

	vars := mux.Vars(r)
	appGUID := vars["guid"]

	authInfo, ok := authorization.InfoFromContext(r.Context())
	if !ok {
		h.logger.Error(nil, "unable to get auth info")
		writeUnknownErrorResponse(w)
		return
	}

	app, err := h.appRepo.FetchApp(ctx, authInfo, appGUID)
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

	err = writeJsonResponse(w, presenter.ForApp(app, h.serverURL), http.StatusOK)
	if err != nil {
		h.logger.Error(err, "Failed to render response", "AppGUID", appGUID)
		writeUnknownErrorResponse(w)
	}
}

func (h *AppHandler) appCreateHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	w.Header().Set("Content-Type", "application/json")

	var payload payloads.AppCreate
	rme := decodeAndValidateJSONPayload(r, &payload)
	if rme != nil {
		writeRequestMalformedErrorResponse(w, rme)
		return
	}

	authInfo, ok := authorization.InfoFromContext(ctx)
	if !ok {
		h.logger.Error(nil, "unable to get auth info")
		writeUnknownErrorResponse(w)
		return
	}
	// TODO: Move this into the action or its own "filter"
	namespaceGUID := payload.Relationships.Space.Data.GUID
	_, err := h.appRepo.FetchNamespace(ctx, authInfo, namespaceGUID)
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

	appRecord, err := h.appRepo.CreateApp(ctx, authInfo, payload.ToAppCreateMessage())
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

	err = writeJsonResponse(w, presenter.ForApp(appRecord, h.serverURL), http.StatusCreated)
	if err != nil {
		h.logger.Error(err, "Failed to render response", "App Name", payload.Name)
		writeUnknownErrorResponse(w)
	}
}

func (h *AppHandler) appListHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	w.Header().Set("Content-Type", "application/json")

	if err := r.ParseForm(); err != nil {
		h.logger.Error(err, "Unable to parse request query parameters")
		writeUnknownErrorResponse(w)
		return
	}

	appListFilter := new(payloads.AppList)
	err := schema.NewDecoder().Decode(appListFilter, r.Form)
	if err != nil {
		switch err.(type) {
		case schema.MultiError:
			multiError := err.(schema.MultiError)
			for _, v := range multiError {
				_, ok := v.(schema.UnknownKeyError)
				if ok {
					h.logger.Info("Unknown key used in Apps filter")
					writeUnknownKeyError(w, appListFilter.SupportedFilterKeys())
					return
				}
			}

			h.logger.Error(err, "Unable to decode request query parameters")
			writeUnknownErrorResponse(w)
		default:
			h.logger.Error(err, "Unable to decode request query parameters")
			writeUnknownErrorResponse(w)
		}
		return
	}

	authInfo, ok := authorization.InfoFromContext(r.Context())
	if !ok {
		h.logger.Error(nil, "unable to get auth info")
		writeUnknownErrorResponse(w)
		return
	}

	appList, err := h.appRepo.FetchAppList(ctx, authInfo, appListFilter.ToMessage())
	if err != nil {
		h.logger.Error(err, "Failed to fetch app(s) from Kubernetes")
		writeUnknownErrorResponse(w)
		return
	}

	err = writeJsonResponse(w, presenter.ForAppList(appList, h.serverURL, *r.URL), http.StatusOK)
	if err != nil {
		h.logger.Error(err, "Failed to render response")
		writeUnknownErrorResponse(w)
	}
}

func (h *AppHandler) appSetCurrentDropletHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	w.Header().Set("Content-Type", "application/json")
	vars := mux.Vars(r)
	appGUID := vars["guid"]

	var payload payloads.AppSetCurrentDroplet
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

	app, err := h.appRepo.FetchApp(ctx, authInfo, appGUID)
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
	droplet, err := h.dropletRepo.FetchDroplet(ctx, authInfo, dropletGUID)
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

	currentDroplet, err := h.appRepo.SetCurrentDroplet(ctx, authInfo, repositories.SetCurrentDropletMessage{
		AppGUID:     appGUID,
		DropletGUID: dropletGUID,
		SpaceGUID:   app.SpaceGUID,
	})
	if err != nil {
		h.logger.Error(err, "Error setting current droplet")
		writeUnknownErrorResponse(w)
		return
	}

	err = writeJsonResponse(w, presenter.ForCurrentDroplet(currentDroplet, h.serverURL), http.StatusOK)
	if err != nil { // untested
		h.logger.Error(err, "Failed to render response")
		writeUnknownErrorResponse(w)
	}
}

func (h *AppHandler) appGetCurrentDropletHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	w.Header().Set("Content-Type", "application/json")
	vars := mux.Vars(r)
	appGUID := vars["guid"]

	authInfo, ok := authorization.InfoFromContext(r.Context())
	if !ok {
		h.logger.Error(nil, "unable to get auth info")
		writeUnknownErrorResponse(w)
		return
	}
	app, err := h.appRepo.FetchApp(ctx, authInfo, appGUID)
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

	droplet, err := h.dropletRepo.FetchDroplet(ctx, authInfo, app.DropletGUID)
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

	err = writeJsonResponse(w, presenter.ForDroplet(droplet, h.serverURL), http.StatusOK)
	if err != nil {
		h.logger.Error(err, "Failed to render response", "dropletGUID", app.DropletGUID)
		writeUnknownErrorResponse(w)
	}
}

func (h *AppHandler) appStartHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	w.Header().Set("Content-Type", "application/json")

	vars := mux.Vars(r)
	appGUID := vars["guid"]

	authInfo, ok := authorization.InfoFromContext(r.Context())
	if !ok {
		h.logger.Error(nil, "unable to get auth info")
		writeUnknownErrorResponse(w)
		return
	}

	app, err := h.appRepo.FetchApp(ctx, authInfo, appGUID)
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

	app, err = h.appRepo.SetAppDesiredState(ctx, authInfo, repositories.SetAppDesiredStateMessage{
		AppGUID:      app.GUID,
		SpaceGUID:    app.SpaceGUID,
		DesiredState: AppStartedState,
	})
	if err != nil {
		h.logger.Error(err, "Failed to update app in Kubernetes", "AppGUID", appGUID)
		writeUnknownErrorResponse(w)
		return
	}

	err = writeJsonResponse(w, presenter.ForApp(app, h.serverURL), http.StatusOK)
	if err != nil {
		h.logger.Error(err, "Failed to render response", "AppGUID", appGUID)
		writeUnknownErrorResponse(w)
	}
}

func (h *AppHandler) appStopHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	w.Header().Set("Content-Type", "application/json")

	vars := mux.Vars(r)
	appGUID := vars["guid"]

	authInfo, ok := authorization.InfoFromContext(r.Context())
	if !ok {
		h.logger.Error(nil, "unable to get auth info")
		writeUnknownErrorResponse(w)
		return
	}

	app, err := h.appRepo.FetchApp(ctx, authInfo, appGUID)
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

	app, err = h.appRepo.SetAppDesiredState(ctx, authInfo, repositories.SetAppDesiredStateMessage{
		AppGUID:      app.GUID,
		SpaceGUID:    app.SpaceGUID,
		DesiredState: AppStoppedState,
	})
	if err != nil {
		h.logger.Error(err, "Failed to update app in Kubernetes", "AppGUID", appGUID)
		writeUnknownErrorResponse(w)
		return
	}

	err = writeJsonResponse(w, presenter.ForApp(app, h.serverURL), http.StatusOK)
	if err != nil {
		h.logger.Error(err, "Failed to render response", "AppGUID", appGUID)
		writeUnknownErrorResponse(w)
	}
}

func (h *AppHandler) getProcessesForAppHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	w.Header().Set("Content-Type", "application/json")

	vars := mux.Vars(r)
	appGUID := vars["guid"]

	authInfo, ok := authorization.InfoFromContext(r.Context())
	if !ok {
		h.logger.Error(nil, "unable to get auth info")
		writeUnknownErrorResponse(w)
		return
	}

	app, err := h.appRepo.FetchApp(ctx, authInfo, appGUID)
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

	fetchProcessesForAppMessage := repositories.FetchProcessListMessage{
		AppGUID:   []string{appGUID},
		SpaceGUID: app.SpaceGUID,
	}

	processList, err := h.processRepo.FetchProcessList(ctx, authInfo, fetchProcessesForAppMessage)
	if err != nil {
		h.logger.Error(err, "Failed to fetch app Process(es) from Kubernetes")
		writeUnknownErrorResponse(w)
		return
	}

	err = writeJsonResponse(w, presenter.ForProcessList(processList, h.serverURL, *r.URL), http.StatusOK)
	if err != nil { // untested
		h.logger.Error(err, "Failed to render response")
		writeUnknownErrorResponse(w)
	}
}

func (h *AppHandler) getRoutesForAppHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	w.Header().Set("Content-Type", "application/json")

	vars := mux.Vars(r)
	appGUID := vars["guid"]

	authInfo, ok := authorization.InfoFromContext(r.Context())
	if !ok {
		h.logger.Error(nil, "unable to get auth info")
		writeUnknownErrorResponse(w)
		return
	}

	app, err := h.appRepo.FetchApp(ctx, authInfo, appGUID)
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

	routes, err := h.lookupAppRouteAndDomainList(ctx, authInfo, app.GUID, app.SpaceGUID)
	if err != nil {
		h.logger.Error(err, "Failed to fetch route or domains from Kubernetes")
		writeUnknownErrorResponse(w)
		return
	}

	err = writeJsonResponse(w, presenter.ForRouteList(routes, h.serverURL, *r.URL), http.StatusOK)
	if err != nil {
		h.logger.Error(err, "Failed to render response")
		writeUnknownErrorResponse(w)
	}
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
		writeRequestMalformedErrorResponse(w, rme)
		return
	}

	authInfo, ok := authorization.InfoFromContext(r.Context())
	if !ok {
		h.logger.Error(nil, "unable to get auth info")
		writeUnknownErrorResponse(w)
		return
	}

	processRecord, err := h.scaleAppProcess(ctx, authInfo, appGUID, processType, payload.ToRecord())
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

	err = writeJsonResponse(w, presenter.ForProcess(processRecord, h.serverURL), http.StatusOK)
	if err != nil {
		h.logger.Error(err, "Failed to render response", "ProcessGUID", appGUID)
		writeUnknownErrorResponse(w)
	}
}

func (h *AppHandler) appRestartHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	w.Header().Set("Content-Type", "application/json")

	vars := mux.Vars(r)
	appGUID := vars["guid"]

	authInfo, ok := authorization.InfoFromContext(r.Context())
	if !ok {
		h.logger.Error(nil, "unable to get auth info")
		writeUnknownErrorResponse(w)
		return
	}

	app, err := h.appRepo.FetchApp(ctx, authInfo, appGUID)
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

	if app.State == repositories.StartedState {
		app, err = h.appRepo.SetAppDesiredState(ctx, authInfo, repositories.SetAppDesiredStateMessage{
			AppGUID:      app.GUID,
			SpaceGUID:    app.SpaceGUID,
			DesiredState: AppStoppedState,
		})
		if err != nil {
			h.logger.Error(err, "Failed to update app in Kubernetes", "AppGUID", appGUID)
			writeUnknownErrorResponse(w)
			return
		}

		var terminated bool
		terminated, err = h.podRepo.WatchForPodsTermination(ctx, authInfo, app.GUID, app.SpaceGUID)
		if err != nil {
			h.logger.Error(err, "Failed to fetch pods for app in Kubernetes", "AppGUID", appGUID)
			writeUnknownErrorResponse(w)
			return
		}

		// Terminated can only be false if the user cancels the context.
		if !terminated {
			h.logger.Error(err, "Timed out waiting for pods to terminate for app", "AppGUID", appGUID)
			writeUnknownErrorResponse(w)
			return
		}
	}

	app, err = h.appRepo.SetAppDesiredState(ctx, authInfo, repositories.SetAppDesiredStateMessage{
		AppGUID:      app.GUID,
		SpaceGUID:    app.SpaceGUID,
		DesiredState: AppStartedState,
	})
	if err != nil {
		h.logger.Error(err, "Failed to update app in Kubernetes", "AppGUID", appGUID)
		writeUnknownErrorResponse(w)
		return
	}

	err = writeJsonResponse(w, presenter.ForApp(app, h.serverURL), http.StatusOK)
	if err != nil {
		h.logger.Error(err, "Failed to render response", "AppGUID", appGUID)
		writeUnknownErrorResponse(w)
	}
}

func (h *AppHandler) lookupAppRouteAndDomainList(ctx context.Context, authInfo authorization.Info, appGUID, spaceGUID string) ([]repositories.RouteRecord, error) {
	routeRecords, err := h.routeRepo.FetchRoutesForApp(ctx, authInfo, appGUID, spaceGUID)
	if err != nil {
		return []repositories.RouteRecord{}, err
	}

	return getDomainsForRoutes(ctx, h.domainRepo, authInfo, routeRecords)
}

func (h *AppHandler) RegisterRoutes(router *mux.Router) {
	router.Path(AppGetEndpoint).Methods("GET").HandlerFunc(h.appGetHandler)
	router.Path(AppListEndpoint).Methods("GET").HandlerFunc(h.appListHandler)
	router.Path(AppCreateEndpoint).Methods("POST").HandlerFunc(h.appCreateHandler)
	router.Path(AppSetCurrentDropletEndpoint).Methods("PATCH").HandlerFunc(h.appSetCurrentDropletHandler)
	router.Path(AppGetCurrentDropletEndpoint).Methods("GET").HandlerFunc(h.appGetCurrentDropletHandler)
	router.Path(AppStartEndpoint).Methods("POST").HandlerFunc(h.appStartHandler)
	router.Path(AppStopEndpoint).Methods("POST").HandlerFunc(h.appStopHandler)
	router.Path(AppRestartEndpoint).Methods("POST").HandlerFunc(h.appRestartHandler)
	router.Path(AppProcessScaleEndpoint).Methods("POST").HandlerFunc(h.appScaleProcessHandler)
	router.Path(AppGetProcessesEndpoint).Methods("GET").HandlerFunc(h.getProcessesForAppHandler)
	router.Path(AppGetRoutesEndpoint).Methods("GET").HandlerFunc(h.getRoutesForAppHandler)
}
