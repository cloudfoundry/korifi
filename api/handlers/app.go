package handlers

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	"code.cloudfoundry.org/korifi/api/authorization"
	apierrors "code.cloudfoundry.org/korifi/api/errors"
	"code.cloudfoundry.org/korifi/api/payloads"
	"code.cloudfoundry.org/korifi/api/presenter"
	"code.cloudfoundry.org/korifi/api/repositories"
	"code.cloudfoundry.org/korifi/api/routing"

	"github.com/go-chi/chi"
	"github.com/go-logr/logr"
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
	PatchAppMetadata(context.Context, authorization.Info, repositories.PatchAppMetadataMessage) (repositories.AppRecord, error)
}

//counterfeiter:generate -o fake -fake-name AppProcessScaler . AppProcessScaler
type AppProcessScaler interface {
	ScaleAppProcess(ctx context.Context, authInfo authorization.Info, appGUID string, processType string, scale repositories.ProcessScaleValues) (repositories.ProcessRecord, error)
}

type App struct {
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

func NewApp(
	serverURL url.URL,
	appRepo CFAppRepository,
	dropletRepo CFDropletRepository,
	processRepo CFProcessRepository,
	routeRepo CFRouteRepository,
	domainRepo CFDomainRepository,
	spaceRepo SpaceRepository,
	appProcessScaler AppProcessScaler,
	decoderValidator *DecoderValidator,
) *App {
	return &App{
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

func (h *App) get(r *http.Request) (*routing.Response, error) {
	appGUID := chi.URLParam(r, "guid")
	authInfo, _ := authorization.InfoFromContext(r.Context())
	logger := logr.FromContextOrDiscard(r.Context()).WithName("handlers.app.get")

	app, err := h.appRepo.GetApp(r.Context(), authInfo, appGUID)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, apierrors.ForbiddenAsNotFound(err), "Failed to fetch app from Kubernetes", "GUID", appGUID)
	}
	return routing.NewResponse(http.StatusOK).WithBody(presenter.ForApp(app, h.serverURL)), nil
}

//nolint:dupl
func (h *App) create(r *http.Request) (*routing.Response, error) {
	authInfo, _ := authorization.InfoFromContext(r.Context())
	logger := logr.FromContextOrDiscard(r.Context()).WithName("handlers.app.create")
	var payload payloads.AppCreate
	if err := h.decoderValidator.DecodeAndValidateJSONPayload(r, &payload); err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "failed to decode json payload")
	}

	spaceGUID := payload.Relationships.Space.Data.GUID
	_, err := h.spaceRepo.GetSpace(r.Context(), authInfo, spaceGUID)
	if err != nil {
		return nil, apierrors.LogAndReturn(
			logger,
			apierrors.AsUnprocessableEntity(err, "Invalid space. Ensure that the space exists and you have access to it.", apierrors.NotFoundError{}, apierrors.ForbiddenError{}),
			"spaceGUID", spaceGUID,
		)
	}

	appRecord, err := h.appRepo.CreateApp(r.Context(), authInfo, payload.ToAppCreateMessage())
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "Failed to create app", "App Name", payload.Name)
	}

	return routing.NewResponse(http.StatusCreated).WithBody(presenter.ForApp(appRecord, h.serverURL)), nil
}

func (h *App) list(r *http.Request) (*routing.Response, error) { //nolint:dupl
	authInfo, _ := authorization.InfoFromContext(r.Context())
	logger := logr.FromContextOrDiscard(r.Context()).WithName("handlers.app.list")

	if err := r.ParseForm(); err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "Unable to parse request query parameters")
	}

	appListFilter := new(payloads.AppList)
	err := payloads.Decode(appListFilter, r.Form)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "Unable to decode request query parameters")
	}

	appList, err := h.appRepo.ListApps(r.Context(), authInfo, appListFilter.ToMessage())
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "Failed to fetch app(s) from Kubernetes")
	}

	return routing.NewResponse(http.StatusOK).WithBody(presenter.ForAppList(appList, h.serverURL, *r.URL)), nil
}

func (h *App) setCurrentDroplet(r *http.Request) (*routing.Response, error) {
	authInfo, _ := authorization.InfoFromContext(r.Context())
	logger := logr.FromContextOrDiscard(r.Context()).WithName("handlers.app.set-current-droplet")
	appGUID := chi.URLParam(r, "guid")

	var payload payloads.AppSetCurrentDroplet
	if err := h.decoderValidator.DecodeAndValidateJSONPayload(r, &payload); err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "failed to decode json payload")
	}

	app, err := h.appRepo.GetApp(r.Context(), authInfo, appGUID)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, apierrors.ForbiddenAsNotFound(err), "Failed to fetch app from Kubernetes", "AppGUID", appGUID)
	}

	dropletGUID := payload.Data.GUID
	droplet, err := h.dropletRepo.GetDroplet(r.Context(), authInfo, dropletGUID)
	if err != nil {
		return nil, apierrors.LogAndReturn(
			logger,
			apierrors.AsUnprocessableEntity(err, invalidDropletMsg, apierrors.ForbiddenError{}, apierrors.NotFoundError{}),
			"Error fetching droplet",
		)
	}

	if droplet.AppGUID != appGUID {
		return nil, apierrors.LogAndReturn(
			logger,
			apierrors.NewUnprocessableEntityError(fmt.Errorf("droplet %s does not belong to app %s", droplet.GUID, appGUID), invalidDropletMsg),
			invalidDropletMsg,
		)
	}

	currentDroplet, err := h.appRepo.SetCurrentDroplet(r.Context(), authInfo, repositories.SetCurrentDropletMessage{
		AppGUID:     appGUID,
		DropletGUID: dropletGUID,
		SpaceGUID:   app.SpaceGUID,
	})
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "Error setting current droplet")
	}

	return routing.NewResponse(http.StatusOK).WithBody(presenter.ForCurrentDroplet(currentDroplet, h.serverURL)), nil
}

func (h *App) getCurrentDroplet(r *http.Request) (*routing.Response, error) {
	authInfo, _ := authorization.InfoFromContext(r.Context())
	logger := logr.FromContextOrDiscard(r.Context()).WithName("handlers.app.get-current-droplet")
	appGUID := chi.URLParam(r, "guid")

	app, err := h.appRepo.GetApp(r.Context(), authInfo, appGUID)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, apierrors.ForbiddenAsNotFound(err), "Failed to fetch app from Kubernetes", "AppGUID", appGUID)
	}

	if app.DropletGUID == "" {
		return nil, apierrors.LogAndReturn(
			logger,
			apierrors.DropletForbiddenAsNotFound(apierrors.NewNotFoundError(nil, repositories.DropletResourceType)),
			"App does not have a current droplet assigned",
			"appGUID", app.GUID,
		)
	}

	droplet, err := h.dropletRepo.GetDroplet(r.Context(), authInfo, app.DropletGUID)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, apierrors.DropletForbiddenAsNotFound(err), "Failed to fetch droplet from Kubernetes", "dropletGUID", app.DropletGUID)
	}

	return routing.NewResponse(http.StatusOK).WithBody(presenter.ForDroplet(droplet, h.serverURL)), nil
}

func (h *App) start(r *http.Request) (*routing.Response, error) {
	authInfo, _ := authorization.InfoFromContext(r.Context())
	logger := logr.FromContextOrDiscard(r.Context()).WithName("handlers.app.start")
	appGUID := chi.URLParam(r, "guid")

	app, err := h.appRepo.GetApp(r.Context(), authInfo, appGUID)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, apierrors.ForbiddenAsNotFound(err), "Failed to fetch app from Kubernetes", "AppGUID", appGUID)
	}
	if app.DropletGUID == "" {
		return nil, apierrors.LogAndReturn(logger, apierrors.NewUnprocessableEntityError(err, "Assign a droplet before starting this app."), "App droplet not set before start", "AppGUID", appGUID)
	}

	app, err = h.appRepo.SetAppDesiredState(r.Context(), authInfo, repositories.SetAppDesiredStateMessage{
		AppGUID:      app.GUID,
		SpaceGUID:    app.SpaceGUID,
		DesiredState: AppStartedState,
	})
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "Failed to update app in Kubernetes", "AppGUID", appGUID)
	}

	return routing.NewResponse(http.StatusOK).WithBody(presenter.ForApp(app, h.serverURL)), nil
}

func (h *App) stop(r *http.Request) (*routing.Response, error) {
	authInfo, _ := authorization.InfoFromContext(r.Context())
	logger := logr.FromContextOrDiscard(r.Context()).WithName("handlers.app.stop")
	appGUID := chi.URLParam(r, "guid")

	app, err := h.appRepo.GetApp(r.Context(), authInfo, appGUID)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, apierrors.ForbiddenAsNotFound(err), "Failed to fetch app from Kubernetes", "AppGUID", appGUID)
	}

	app, err = h.appRepo.SetAppDesiredState(r.Context(), authInfo, repositories.SetAppDesiredStateMessage{
		AppGUID:      app.GUID,
		SpaceGUID:    app.SpaceGUID,
		DesiredState: AppStoppedState,
	})
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "Failed to update app in Kubernetes", "AppGUID", appGUID)
	}

	return routing.NewResponse(http.StatusOK).WithBody(presenter.ForApp(app, h.serverURL)), nil
}

func (h *App) getProcesses(r *http.Request) (*routing.Response, error) {
	authInfo, _ := authorization.InfoFromContext(r.Context())
	logger := logr.FromContextOrDiscard(r.Context()).WithName("handlers.app.get-processes")
	appGUID := chi.URLParam(r, "guid")

	app, err := h.appRepo.GetApp(r.Context(), authInfo, appGUID)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, apierrors.ForbiddenAsNotFound(err), "Failed to fetch app from Kubernetes", "AppGUID", appGUID)
	}

	fetchProcessesForAppMessage := repositories.ListProcessesMessage{
		AppGUIDs:  []string{appGUID},
		SpaceGUID: app.SpaceGUID,
	}

	processList, err := h.processRepo.ListProcesses(r.Context(), authInfo, fetchProcessesForAppMessage)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "Failed to fetch app Process(es) from Kubernetes")
	}

	return routing.NewResponse(http.StatusOK).WithBody(presenter.ForProcessList(processList, h.serverURL, *r.URL)), nil
}

func (h *App) getRoutes(r *http.Request) (*routing.Response, error) {
	authInfo, _ := authorization.InfoFromContext(r.Context())
	logger := logr.FromContextOrDiscard(r.Context()).WithName("handlers.app.get-routes")
	appGUID := chi.URLParam(r, "guid")

	app, err := h.appRepo.GetApp(r.Context(), authInfo, appGUID)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, apierrors.ForbiddenAsNotFound(err), "Failed to fetch app from Kubernetes", "AppGUID", appGUID)
	}

	routes, err := h.lookupAppRouteAndDomainList(r.Context(), authInfo, app.GUID, app.SpaceGUID)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "Failed to fetch route or domains from Kubernetes")
	}

	return routing.NewResponse(http.StatusOK).WithBody(presenter.ForRouteList(routes, h.serverURL, *r.URL)), nil
}

func (h *App) scaleProcess(r *http.Request) (*routing.Response, error) {
	authInfo, _ := authorization.InfoFromContext(r.Context())
	logger := logr.FromContextOrDiscard(r.Context()).WithName("handlers.app.scale-process")
	appGUID := chi.URLParam(r, "guid")
	processType := chi.URLParam(r, "processType")

	var payload payloads.ProcessScale
	if err := h.decoderValidator.DecodeAndValidateJSONPayload(r, &payload); err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "failed to decode json payload")
	}

	processRecord, err := h.appProcessScaler.ScaleAppProcess(r.Context(), authInfo, appGUID, processType, payload.ToRecord())
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "Failed due to error from Kubernetes", "appGUID", appGUID)
	}

	return routing.NewResponse(http.StatusOK).WithBody(presenter.ForProcess(processRecord, h.serverURL)), nil
}

func (h *App) restart(r *http.Request) (*routing.Response, error) {
	authInfo, _ := authorization.InfoFromContext(r.Context())
	logger := logr.FromContextOrDiscard(r.Context()).WithName("handlers.app.restart")
	appGUID := chi.URLParam(r, "guid")

	app, err := h.appRepo.GetApp(r.Context(), authInfo, appGUID)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, apierrors.ForbiddenAsNotFound(err), "Failed to fetch app from Kubernetes", "AppGUID", appGUID)
	}

	if app.DropletGUID == "" {
		return nil, apierrors.LogAndReturn(
			logger,
			apierrors.NewUnprocessableEntityError(fmt.Errorf("app %s has no droplet set", app.GUID), "Assign a droplet before starting this app."),
			"App droplet not set before start",
			"AppGUID", appGUID,
		)
	}

	if app.State == repositories.StartedState {
		app, err = h.appRepo.SetAppDesiredState(r.Context(), authInfo, repositories.SetAppDesiredStateMessage{
			AppGUID:      app.GUID,
			SpaceGUID:    app.SpaceGUID,
			DesiredState: AppStoppedState,
		})
		if err != nil {
			return nil, apierrors.LogAndReturn(logger, err, "Failed to update app in Kubernetes", "AppGUID", appGUID)
		}
	}

	app, err = h.appRepo.SetAppDesiredState(r.Context(), authInfo, repositories.SetAppDesiredStateMessage{
		AppGUID:      app.GUID,
		SpaceGUID:    app.SpaceGUID,
		DesiredState: AppStartedState,
	})
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "Failed to update app in Kubernetes", "AppGUID", appGUID)
	}

	return routing.NewResponse(http.StatusOK).WithBody(presenter.ForApp(app, h.serverURL)), nil
}

func (h *App) delete(r *http.Request) (*routing.Response, error) {
	authInfo, _ := authorization.InfoFromContext(r.Context())
	logger := logr.FromContextOrDiscard(r.Context()).WithName("handlers.app.delete")
	appGUID := chi.URLParam(r, "guid")

	app, err := h.appRepo.GetApp(r.Context(), authInfo, appGUID)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, apierrors.ForbiddenAsNotFound(err), "Failed to fetch app from Kubernetes", "AppGUID", appGUID)
	}

	err = h.appRepo.DeleteApp(r.Context(), authInfo, repositories.DeleteAppMessage{
		AppGUID:   appGUID,
		SpaceGUID: app.SpaceGUID,
	})
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "Failed to delete app", "AppGUID", appGUID)
	}

	return routing.NewResponse(http.StatusAccepted).WithHeader("Location", presenter.JobURLForRedirects(appGUID, presenter.AppDeleteOperation, h.serverURL)), nil
}

func (h *App) lookupAppRouteAndDomainList(ctx context.Context, authInfo authorization.Info, appGUID, spaceGUID string) ([]repositories.RouteRecord, error) {
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

func (h *App) updateEnvVars(r *http.Request) (*routing.Response, error) {
	authInfo, _ := authorization.InfoFromContext(r.Context())
	logger := logr.FromContextOrDiscard(r.Context()).WithName("handlers.app.update-env-vars")
	appGUID := chi.URLParam(r, "guid")

	var payload payloads.AppPatchEnvVars
	if err := h.decoderValidator.DecodeAndValidateJSONPayload(r, &payload); err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "failed to decode payload")
	}

	app, err := h.appRepo.GetApp(r.Context(), authInfo, appGUID)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, apierrors.ForbiddenAsNotFound(err), "Failed to fetch app from Kubernetes", "AppGUID", appGUID)
	}

	envVarsRecord, err := h.appRepo.PatchAppEnvVars(r.Context(), authInfo, payload.ToMessage(appGUID, app.SpaceGUID))
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "Error updating app environment variables")
	}

	return routing.NewResponse(http.StatusOK).WithBody(presenter.ForAppEnvVars(envVarsRecord, h.serverURL)), nil
}

func (h *App) getEnvironment(r *http.Request) (*routing.Response, error) {
	authInfo, _ := authorization.InfoFromContext(r.Context())
	logger := logr.FromContextOrDiscard(r.Context()).WithName("handlers.app.get-environment")
	appGUID := chi.URLParam(r, "guid")

	appEnvRecord, err := h.appRepo.GetAppEnv(r.Context(), authInfo, appGUID)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "Failed to fetch app environment variables", "AppGUID", appGUID)
	}

	return routing.NewResponse(http.StatusOK).WithBody(presenter.ForAppEnv(appEnvRecord)), nil
}

func (h *App) getProcess(r *http.Request) (*routing.Response, error) {
	authInfo, _ := authorization.InfoFromContext(r.Context())
	logger := logr.FromContextOrDiscard(r.Context()).WithName("handlers.app.get-process")
	appGUID := chi.URLParam(r, "guid")
	processType := chi.URLParam(r, "type")

	app, err := h.appRepo.GetApp(r.Context(), authInfo, appGUID)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, apierrors.ForbiddenAsNotFound(err), "Failed to fetch app from Kubernetes", "AppGUID", appGUID)
	}

	process, err := h.processRepo.GetProcessByAppTypeAndSpace(r.Context(), authInfo, appGUID, processType, app.SpaceGUID)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "Failed to fetch process from Kubernetes", "AppGUID", appGUID)
	}

	return routing.NewResponse(http.StatusOK).WithBody(presenter.ForProcess(process, h.serverURL)), nil
}

//nolint:dupl
func (h *App) update(r *http.Request) (*routing.Response, error) {
	authInfo, _ := authorization.InfoFromContext(r.Context())
	logger := logr.FromContextOrDiscard(r.Context()).WithName("handlers.app.update")
	appGUID := chi.URLParam(r, "guid")

	app, err := h.appRepo.GetApp(r.Context(), authInfo, appGUID)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, apierrors.ForbiddenAsNotFound(err), "Failed to fetch app from Kubernetes", "AppGUID", appGUID)
	}

	var payload payloads.AppPatch
	if err = h.decoderValidator.DecodeAndValidateJSONPayload(r, &payload); err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "failed to decode payload")
	}

	app, err = h.appRepo.PatchAppMetadata(r.Context(), authInfo, payload.ToMessage(appGUID, app.SpaceGUID))
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "Failed to patch app metadata", "AppGUID", appGUID)
	}
	return routing.NewResponse(http.StatusOK).WithBody(presenter.ForApp(app, h.serverURL)), nil
}

func (h *App) RegisterRoutes(router *chi.Mux) {
	router.Method("GET", AppPath, routing.Handler(h.get))
	router.Method("GET", AppsPath, routing.Handler(h.list))
	router.Method("POST", AppsPath, routing.Handler(h.create))
	router.Method("PATCH", AppCurrentDropletRelationshipPath, routing.Handler(h.setCurrentDroplet))
	router.Method("GET", AppCurrentDropletPath, routing.Handler(h.getCurrentDroplet))
	router.Method("POST", AppStartPath, routing.Handler(h.start))
	router.Method("POST", AppStopPath, routing.Handler(h.stop))
	router.Method("POST", AppRestartPath, routing.Handler(h.restart))
	router.Method("POST", AppProcessScalePath, routing.Handler(h.scaleProcess))
	router.Method("GET", AppProcessesPath, routing.Handler(h.getProcesses))
	router.Method("GET", AppProcessByTypePath, routing.Handler(h.getProcess))
	router.Method("GET", AppRoutesPath, routing.Handler(h.getRoutes))
	router.Method("DELETE", AppPath, routing.Handler(h.delete))
	router.Method("PATCH", AppEnvVarsPath, routing.Handler(h.updateEnvVars))
	router.Method("GET", AppEnvPath, routing.Handler(h.getEnvironment))
	router.Method("PATCH", AppPath, routing.Handler(h.update))
}
