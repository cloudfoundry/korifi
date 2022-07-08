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

	"github.com/go-logr/logr"
	"github.com/gorilla/mux"
	ctrl "sigs.k8s.io/controller-runtime"
)

const (
	AppsPath                          = "/v3/apps"
	AppPath                           = "/v3/apps/{guid}"
	AppCurrentDropletRelationshipPath = "/v3/apps/{guid}/relationships/current_droplet"
	AppCurrentDropletPath             = "/v3/apps/{guid}/droplets/current"
	AppProcessesPath                  = "/v3/apps/{guid}/processes"
	AppProcessByTypePath              = "/v3/apps/{guid}/processes/{type}"
	AppProcessScalePath               = "/v3/apps/{guid}/processes/{processType}/actions/scale"
	AppRoutesPath                     = "/v3/apps/{guid}/routes"
	AppStartPath                      = "/v3/apps/{guid}/actions/start"
	AppStopPath                       = "/v3/apps/{guid}/actions/stop"
	AppRestartPath                    = "/v3/apps/{guid}/actions/restart"
	AppEnvVarsPath                    = "/v3/apps/{guid}/environment_variables"
	AppEnvPath                        = "/v3/apps/{guid}/env"
	invalidDropletMsg                 = "Unable to assign current droplet. Ensure the droplet exists and belongs to this app."

	AppStartedState = "STARTED"
	AppStoppedState = "STOPPED"
)

//counterfeiter:generate -o fake -fake-name CFAppRepository . CFAppRepository
type CFAppRepository interface {
	GetApp(context.Context, authorization.Info, string) (repositories.AppRecord, error)
	ListApps(context.Context, authorization.Info, repositories.ListAppsMessage) ([]repositories.AppRecord, error)
	PatchAppEnvVars(context.Context, authorization.Info, repositories.PatchAppEnvVarsMessage) (repositories.AppEnvVarsRecord, error)
	CreateApp(context.Context, authorization.Info, repositories.CreateAppMessage) (repositories.AppRecord, error)
	SetCurrentDroplet(context.Context, authorization.Info, repositories.SetCurrentDropletMessage) (repositories.CurrentDropletRecord, error)
	SetAppDesiredState(context.Context, authorization.Info, repositories.SetAppDesiredStateMessage) (repositories.AppRecord, error)
	DeleteApp(context.Context, authorization.Info, repositories.DeleteAppMessage) error
	GetAppEnv(context.Context, authorization.Info, string) (repositories.AppEnvRecord, error)
}

//counterfeiter:generate -o fake -fake-name AppProcessScaler . AppProcessScaler
type AppProcessScaler interface {
	ScaleAppProcess(ctx context.Context, authInfo authorization.Info, appGUID string, processType string, scale repositories.ProcessScaleValues) (repositories.ProcessRecord, error)
}

type AppHandler struct {
	handlerWrapper   *AuthAwareHandlerFuncWrapper
	serverURL        url.URL
	appRepo          CFAppRepository
	dropletRepo      CFDropletRepository
	processRepo      CFProcessRepository
	routeRepo        CFRouteRepository
	domainRepo       CFDomainRepository
	spaceRepo        SpaceRepository
	appProcessScaler AppProcessScaler
	decoderValidator *DecoderValidator
}

func NewAppHandler(
	serverURL url.URL,
	appRepo CFAppRepository,
	dropletRepo CFDropletRepository,
	processRepo CFProcessRepository,
	routeRepo CFRouteRepository,
	domainRepo CFDomainRepository,
	spaceRepo SpaceRepository,
	appProcessScaler AppProcessScaler,
	decoderValidator *DecoderValidator,
) *AppHandler {
	return &AppHandler{
		handlerWrapper:   NewAuthAwareHandlerFuncWrapper(ctrl.Log.WithName("AppHandler")),
		serverURL:        serverURL,
		appRepo:          appRepo,
		dropletRepo:      dropletRepo,
		processRepo:      processRepo,
		routeRepo:        routeRepo,
		domainRepo:       domainRepo,
		decoderValidator: decoderValidator,
		spaceRepo:        spaceRepo,
		appProcessScaler: appProcessScaler,
	}
}

func (h *AppHandler) appGetHandler(ctx context.Context, logger logr.Logger, authInfo authorization.Info, r *http.Request) (*HandlerResponse, error) {
	vars := mux.Vars(r)
	appGUID := vars["guid"]

	app, err := h.appRepo.GetApp(ctx, authInfo, appGUID)
	if err != nil {
		logger.Error(err, "Failed to fetch app from Kubernetes", "AppGUID", appGUID)
		return nil, apierrors.ForbiddenAsNotFound(err)
	}
	return NewHandlerResponse(http.StatusOK).WithBody(presenter.ForApp(app, h.serverURL)), nil
}

//nolint:dupl
func (h *AppHandler) appCreateHandler(ctx context.Context, logger logr.Logger, authInfo authorization.Info, r *http.Request) (*HandlerResponse, error) {
	var payload payloads.AppCreate
	if err := h.decoderValidator.DecodeAndValidateJSONPayload(r, &payload); err != nil {
		return nil, err
	}

	spaceGUID := payload.Relationships.Space.Data.GUID
	_, err := h.spaceRepo.GetSpace(ctx, authInfo, spaceGUID)
	if err != nil {
		logger.Error(err, "Failed to fetch space from Kubernetes", "spaceGUID", spaceGUID)
		return nil, apierrors.AsUnprocessableEntity(err, "Invalid space. Ensure that the space exists and you have access to it.", apierrors.NotFoundError{}, apierrors.ForbiddenError{})
	}

	appRecord, err := h.appRepo.CreateApp(ctx, authInfo, payload.ToAppCreateMessage())
	if err != nil {
		logger.Error(err, "Failed to create app", "App Name", payload.Name)
		return nil, err
	}

	return NewHandlerResponse(http.StatusCreated).WithBody(presenter.ForApp(appRecord, h.serverURL)), nil
}

func (h *AppHandler) appListHandler(ctx context.Context, logger logr.Logger, authInfo authorization.Info, r *http.Request) (*HandlerResponse, error) { //nolint:dupl

	if err := r.ParseForm(); err != nil {
		logger.Error(err, "Unable to parse request query parameters")
		return nil, err
	}

	appListFilter := new(payloads.AppList)
	err := payloads.Decode(appListFilter, r.Form)
	if err != nil {
		logger.Error(err, "Unable to decode request query parameters")
		return nil, err
	}

	appList, err := h.appRepo.ListApps(ctx, authInfo, appListFilter.ToMessage())
	if err != nil {
		logger.Error(err, "Failed to fetch app(s) from Kubernetes")
		return nil, err
	}

	return NewHandlerResponse(http.StatusOK).WithBody(presenter.ForAppList(appList, h.serverURL, *r.URL)), nil
}

func (h *AppHandler) appSetCurrentDropletHandler(ctx context.Context, logger logr.Logger, authInfo authorization.Info, r *http.Request) (*HandlerResponse, error) {
	vars := mux.Vars(r)
	appGUID := vars["guid"]

	var payload payloads.AppSetCurrentDroplet
	if err := h.decoderValidator.DecodeAndValidateJSONPayload(r, &payload); err != nil {
		return nil, err
	}

	app, err := h.appRepo.GetApp(ctx, authInfo, appGUID)
	if err != nil {
		logger.Error(err, "Failed to fetch app from Kubernetes", "AppGUID", appGUID)
		return nil, apierrors.ForbiddenAsNotFound(err)
	}

	dropletGUID := payload.Data.GUID
	droplet, err := h.dropletRepo.GetDroplet(ctx, authInfo, dropletGUID)
	if err != nil {
		logger.Error(err, "Error fetching droplet")
		return nil, apierrors.AsUnprocessableEntity(err, invalidDropletMsg, apierrors.ForbiddenError{}, apierrors.NotFoundError{})
	}

	if droplet.AppGUID != appGUID {
		return nil, apierrors.NewUnprocessableEntityError(fmt.Errorf("droplet %s does not belong to app %s", droplet.GUID, appGUID), invalidDropletMsg)
	}

	currentDroplet, err := h.appRepo.SetCurrentDroplet(ctx, authInfo, repositories.SetCurrentDropletMessage{
		AppGUID:     appGUID,
		DropletGUID: dropletGUID,
		SpaceGUID:   app.SpaceGUID,
	})
	if err != nil {
		logger.Error(err, "Error setting current droplet")
		return nil, err
	}

	return NewHandlerResponse(http.StatusOK).WithBody(presenter.ForCurrentDroplet(currentDroplet, h.serverURL)), nil
}

func (h *AppHandler) appGetCurrentDropletHandler(ctx context.Context, logger logr.Logger, authInfo authorization.Info, r *http.Request) (*HandlerResponse, error) {
	vars := mux.Vars(r)
	appGUID := vars["guid"]

	app, err := h.appRepo.GetApp(ctx, authInfo, appGUID)
	if err != nil {
		logger.Error(err, "Failed to fetch app from Kubernetes", "AppGUID", appGUID)
		return nil, apierrors.ForbiddenAsNotFound(err)
	}

	if app.DropletGUID == "" {
		logger.Info("App does not have a current droplet assigned", "appGUID", app.GUID)
		return nil, apierrors.DropletForbiddenAsNotFound(apierrors.NewNotFoundError(err, repositories.DropletResourceType))
	}

	droplet, err := h.dropletRepo.GetDroplet(ctx, authInfo, app.DropletGUID)
	if err != nil {
		logger.Error(err, "Failed to fetch droplet from Kubernetes", "dropletGUID", app.DropletGUID)
		return nil, apierrors.DropletForbiddenAsNotFound(err)
	}

	return NewHandlerResponse(http.StatusOK).WithBody(presenter.ForDroplet(droplet, h.serverURL)), nil
}

func (h *AppHandler) appStartHandler(ctx context.Context, logger logr.Logger, authInfo authorization.Info, r *http.Request) (*HandlerResponse, error) {
	vars := mux.Vars(r)
	appGUID := vars["guid"]

	app, err := h.appRepo.GetApp(ctx, authInfo, appGUID)
	if err != nil {
		logger.Error(err, "Failed to fetch app from Kubernetes", "AppGUID", appGUID)
		return nil, apierrors.ForbiddenAsNotFound(err)
	}
	if app.DropletGUID == "" {
		logger.Info("App droplet not set before start", "AppGUID", appGUID)
		return nil, apierrors.NewUnprocessableEntityError(err, "Assign a droplet before starting this app.")
	}

	app, err = h.appRepo.SetAppDesiredState(ctx, authInfo, repositories.SetAppDesiredStateMessage{
		AppGUID:      app.GUID,
		SpaceGUID:    app.SpaceGUID,
		DesiredState: AppStartedState,
	})
	if err != nil {
		logger.Error(err, "Failed to update app in Kubernetes", "AppGUID", appGUID)
		return nil, err
	}

	return NewHandlerResponse(http.StatusOK).WithBody(presenter.ForApp(app, h.serverURL)), nil
}

func (h *AppHandler) appStopHandler(ctx context.Context, logger logr.Logger, authInfo authorization.Info, r *http.Request) (*HandlerResponse, error) {
	vars := mux.Vars(r)
	appGUID := vars["guid"]

	app, err := h.appRepo.GetApp(ctx, authInfo, appGUID)
	if err != nil {
		logger.Error(err, "Failed to fetch app from Kubernetes", "AppGUID", appGUID)
		return nil, apierrors.ForbiddenAsNotFound(err)
	}

	app, err = h.appRepo.SetAppDesiredState(ctx, authInfo, repositories.SetAppDesiredStateMessage{
		AppGUID:      app.GUID,
		SpaceGUID:    app.SpaceGUID,
		DesiredState: AppStoppedState,
	})
	if err != nil {
		logger.Error(err, "Failed to update app in Kubernetes", "AppGUID", appGUID)
		return nil, err
	}

	return NewHandlerResponse(http.StatusOK).WithBody(presenter.ForApp(app, h.serverURL)), nil
}

func (h *AppHandler) getProcessesForAppHandler(ctx context.Context, logger logr.Logger, authInfo authorization.Info, r *http.Request) (*HandlerResponse, error) {
	vars := mux.Vars(r)
	appGUID := vars["guid"]

	app, err := h.appRepo.GetApp(ctx, authInfo, appGUID)
	if err != nil {
		logger.Error(err, "Failed to fetch app from Kubernetes", "AppGUID", appGUID)
		return nil, apierrors.ForbiddenAsNotFound(err)
	}

	fetchProcessesForAppMessage := repositories.ListProcessesMessage{
		AppGUIDs:  []string{appGUID},
		SpaceGUID: app.SpaceGUID,
	}

	processList, err := h.processRepo.ListProcesses(ctx, authInfo, fetchProcessesForAppMessage)
	if err != nil {
		logger.Error(err, "Failed to fetch app Process(es) from Kubernetes")
		return nil, err
	}

	return NewHandlerResponse(http.StatusOK).WithBody(presenter.ForProcessList(processList, h.serverURL, *r.URL)), nil
}

func (h *AppHandler) getRoutesForAppHandler(ctx context.Context, logger logr.Logger, authInfo authorization.Info, r *http.Request) (*HandlerResponse, error) {
	vars := mux.Vars(r)
	appGUID := vars["guid"]

	app, err := h.appRepo.GetApp(ctx, authInfo, appGUID)
	if err != nil {
		logger.Error(err, "Failed to fetch app from Kubernetes", "AppGUID", appGUID)
		return nil, apierrors.ForbiddenAsNotFound(err)
	}

	routes, err := h.lookupAppRouteAndDomainList(ctx, authInfo, app.GUID, app.SpaceGUID)
	if err != nil {
		logger.Error(err, "Failed to fetch route or domains from Kubernetes")
		return nil, err
	}

	return NewHandlerResponse(http.StatusOK).WithBody(presenter.ForRouteList(routes, h.serverURL, *r.URL)), nil
}

func (h *AppHandler) appScaleProcessHandler(ctx context.Context, logger logr.Logger, authInfo authorization.Info, r *http.Request) (*HandlerResponse, error) {
	vars := mux.Vars(r)
	appGUID := vars["guid"]
	processType := vars["processType"]

	var payload payloads.ProcessScale
	if err := h.decoderValidator.DecodeAndValidateJSONPayload(r, &payload); err != nil {
		return nil, err
	}

	processRecord, err := h.appProcessScaler.ScaleAppProcess(ctx, authInfo, appGUID, processType, payload.ToRecord())
	if err != nil {
		logger.Error(err, "Failed due to error from Kubernetes", "appGUID", appGUID)
		return nil, err
	}

	return NewHandlerResponse(http.StatusOK).WithBody(presenter.ForProcess(processRecord, h.serverURL)), nil
}

func (h *AppHandler) appRestartHandler(ctx context.Context, logger logr.Logger, authInfo authorization.Info, r *http.Request) (*HandlerResponse, error) {
	vars := mux.Vars(r)
	appGUID := vars["guid"]

	app, err := h.appRepo.GetApp(ctx, authInfo, appGUID)
	if err != nil {
		logger.Error(err, "Failed to fetch app from Kubernetes", "AppGUID", appGUID)
		return nil, apierrors.ForbiddenAsNotFound(err)
	}

	if app.DropletGUID == "" {
		logger.Info("App droplet not set before start", "AppGUID", appGUID)
		return nil, apierrors.NewUnprocessableEntityError(fmt.Errorf("app %s has no droplet set", app.GUID), "Assign a droplet before starting this app.")
	}

	if app.State == repositories.StartedState {
		app, err = h.appRepo.SetAppDesiredState(ctx, authInfo, repositories.SetAppDesiredStateMessage{
			AppGUID:      app.GUID,
			SpaceGUID:    app.SpaceGUID,
			DesiredState: AppStoppedState,
		})
		if err != nil {
			logger.Error(err, "Failed to update app in Kubernetes", "AppGUID", appGUID)
			return nil, err
		}
	}

	app, err = h.appRepo.SetAppDesiredState(ctx, authInfo, repositories.SetAppDesiredStateMessage{
		AppGUID:      app.GUID,
		SpaceGUID:    app.SpaceGUID,
		DesiredState: AppStartedState,
	})
	if err != nil {
		logger.Error(err, "Failed to update app in Kubernetes", "AppGUID", appGUID)
		return nil, err
	}

	return NewHandlerResponse(http.StatusOK).WithBody(presenter.ForApp(app, h.serverURL)), nil
}

func (h *AppHandler) appDeleteHandler(ctx context.Context, logger logr.Logger, authInfo authorization.Info, r *http.Request) (*HandlerResponse, error) {
	vars := mux.Vars(r)
	appGUID := vars["guid"]

	app, err := h.appRepo.GetApp(ctx, authInfo, appGUID)
	if err != nil {
		logger.Error(err, "Failed to fetch app from Kubernetes", "AppGUID", appGUID)
		return nil, apierrors.ForbiddenAsNotFound(err)
	}

	err = h.appRepo.DeleteApp(ctx, authInfo, repositories.DeleteAppMessage{
		AppGUID:   appGUID,
		SpaceGUID: app.SpaceGUID,
	})
	if err != nil {
		logger.Error(err, "Failed to delete app", "AppGUID", appGUID)
		return nil, err
	}

	return NewHandlerResponse(http.StatusAccepted).WithHeader("Location", presenter.JobURLForRedirects(appGUID, presenter.AppDeleteOperation, h.serverURL)), nil
}

func (h *AppHandler) lookupAppRouteAndDomainList(ctx context.Context, authInfo authorization.Info, appGUID, spaceGUID string) ([]repositories.RouteRecord, error) {
	routeRecords, err := h.routeRepo.ListRoutesForApp(ctx, authInfo, appGUID, spaceGUID)
	if err != nil {
		return []repositories.RouteRecord{}, err
	}

	return getDomainsForRoutes(ctx, h.domainRepo, authInfo, routeRecords)
}

func getDomainsForRoutes(ctx context.Context, domainRepo CFDomainRepository, authInfo authorization.Info, routeRecords []repositories.RouteRecord) ([]repositories.RouteRecord, error) {
	domainGUIDToDomainRecord := make(map[string]repositories.DomainRecord)
	for i, routeRecord := range routeRecords {
		currentDomainGUID := routeRecord.Domain.GUID
		domainRecord, has := domainGUIDToDomainRecord[currentDomainGUID]
		if !has {
			var err error
			domainRecord, err = domainRepo.GetDomain(ctx, authInfo, currentDomainGUID)
			if err != nil {
				// err = errors.New("resource not found for route's specified domain ref")
				return []repositories.RouteRecord{}, err
			}
			domainGUIDToDomainRecord[currentDomainGUID] = domainRecord
		}
		routeRecords[i] = routeRecord.UpdateDomainRef(domainRecord)
	}

	return routeRecords, nil
}

func (h *AppHandler) appPatchEnvVarsHandler(ctx context.Context, logger logr.Logger, authInfo authorization.Info, r *http.Request) (*HandlerResponse, error) {
	vars := mux.Vars(r)
	appGUID := vars["guid"]

	var payload payloads.AppPatchEnvVars
	if err := h.decoderValidator.DecodeAndValidateJSONPayload(r, &payload); err != nil {
		return nil, err
	}

	app, err := h.appRepo.GetApp(ctx, authInfo, appGUID)
	if err != nil {
		logger.Error(err, "Failed to fetch app from Kubernetes", "AppGUID", appGUID)
		return nil, apierrors.ForbiddenAsNotFound(err)
	}

	envVarsRecord, err := h.appRepo.PatchAppEnvVars(ctx, authInfo, payload.ToMessage(appGUID, app.SpaceGUID))
	if err != nil {
		logger.Error(err, "Error updating app environment variables")
		return nil, err
	}

	return NewHandlerResponse(http.StatusOK).WithBody(presenter.ForAppEnvVars(envVarsRecord, h.serverURL)), nil
}

func (h *AppHandler) appGetEnvHandler(ctx context.Context, logger logr.Logger, authInfo authorization.Info, r *http.Request) (*HandlerResponse, error) {
	vars := mux.Vars(r)
	appGUID := vars["guid"]

	appEnvRecord, err := h.appRepo.GetAppEnv(ctx, authInfo, appGUID)
	if err != nil {
		logger.Error(err, "Failed to fetch app environment variables", "AppGUID", appGUID)
		return nil, err
	}

	return NewHandlerResponse(http.StatusOK).WithBody(presenter.ForAppEnv(appEnvRecord)), nil
}

func (h *AppHandler) getProcessByTypeForAppHander(ctx context.Context, logger logr.Logger, authInfo authorization.Info, r *http.Request) (*HandlerResponse, error) {
	vars := mux.Vars(r)
	appGUID := vars["guid"]
	processType := vars["type"]

	app, err := h.appRepo.GetApp(ctx, authInfo, appGUID)
	if err != nil {
		logger.Error(err, "Failed to fetch app from Kubernetes", "AppGUID", appGUID)
		return nil, apierrors.ForbiddenAsNotFound(err)
	}

	process, err := h.processRepo.GetProcessByAppTypeAndSpace(ctx, authInfo, appGUID, processType, app.SpaceGUID)
	if err != nil {
		logger.Error(err, "Failed to fetch process from Kubernetes", "AppGUID", appGUID)
		return nil, err
	}

	return NewHandlerResponse(http.StatusOK).WithBody(presenter.ForProcess(process, h.serverURL)), nil
}

func (h *AppHandler) RegisterRoutes(router *mux.Router) {
	router.Path(AppPath).Methods("GET").HandlerFunc(h.handlerWrapper.Wrap(h.appGetHandler))
	router.Path(AppsPath).Methods("GET").HandlerFunc(h.handlerWrapper.Wrap(h.appListHandler))
	router.Path(AppsPath).Methods("POST").HandlerFunc(h.handlerWrapper.Wrap(h.appCreateHandler))
	router.Path(AppCurrentDropletRelationshipPath).Methods("PATCH").HandlerFunc(h.handlerWrapper.Wrap(h.appSetCurrentDropletHandler))
	router.Path(AppCurrentDropletPath).Methods("GET").HandlerFunc(h.handlerWrapper.Wrap(h.appGetCurrentDropletHandler))
	router.Path(AppStartPath).Methods("POST").HandlerFunc(h.handlerWrapper.Wrap(h.appStartHandler))
	router.Path(AppStopPath).Methods("POST").HandlerFunc(h.handlerWrapper.Wrap(h.appStopHandler))
	router.Path(AppRestartPath).Methods("POST").HandlerFunc(h.handlerWrapper.Wrap(h.appRestartHandler))
	router.Path(AppProcessScalePath).Methods("POST").HandlerFunc(h.handlerWrapper.Wrap(h.appScaleProcessHandler))
	router.Path(AppProcessesPath).Methods("GET").HandlerFunc(h.handlerWrapper.Wrap(h.getProcessesForAppHandler))
	router.Path(AppProcessByTypePath).Methods("GET").HandlerFunc(h.handlerWrapper.Wrap(h.getProcessByTypeForAppHander))
	router.Path(AppRoutesPath).Methods("GET").HandlerFunc(h.handlerWrapper.Wrap(h.getRoutesForAppHandler))
	router.Path(AppPath).Methods("DELETE").HandlerFunc(h.handlerWrapper.Wrap(h.appDeleteHandler))
	router.Path(AppEnvVarsPath).Methods("PATCH").HandlerFunc(h.handlerWrapper.Wrap(h.appPatchEnvVarsHandler))
	router.Path(AppEnvPath).Methods("GET").HandlerFunc(h.handlerWrapper.Wrap(h.appGetEnvHandler))
}
