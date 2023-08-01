package handlers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"time"

	"code.cloudfoundry.org/korifi/api/authorization"
	apierrors "code.cloudfoundry.org/korifi/api/errors"
	"code.cloudfoundry.org/korifi/api/payloads"
	"code.cloudfoundry.org/korifi/api/presenter"
	"code.cloudfoundry.org/korifi/api/repositories"
	"code.cloudfoundry.org/korifi/api/routing"
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"

	"github.com/go-logr/logr"
)

const (
	AppsPath                          = "/v3/apps"
	AppPath                           = "/v3/apps/{guid}"
	AppCurrentDropletRelationshipPath = "/v3/apps/{guid}/relationships/current_droplet"
	AppCurrentDropletPath             = "/v3/apps/{guid}/droplets/current"
	AppProcessesPath                  = "/v3/apps/{guid}/processes"
	AppProcessByTypePath              = "/v3/apps/{guid}/processes/{type}"
	AppProcessStatsByTypePath         = "/v3/apps/{guid}/processes/{type}/stats"
	AppProcessScalePath               = "/v3/apps/{guid}/processes/{processType}/actions/scale"
	AppRoutesPath                     = "/v3/apps/{guid}/routes"
	AppStartPath                      = "/v3/apps/{guid}/actions/start"
	AppStopPath                       = "/v3/apps/{guid}/actions/stop"
	AppRestartPath                    = "/v3/apps/{guid}/actions/restart"
	AppEnvVarsPath                    = "/v3/apps/{guid}/environment_variables"
	AppEnvPath                        = "/v3/apps/{guid}/env"
	AppFeaturePath                    = "/v3/apps/{guid}/features/{name}"
	AppPackagesPath                   = "/v3/apps/{guid}/packages"
	AppSSHEnabledPath                 = "/v3/apps/{guid}/ssh_enabled"
	AppBuildsPath                     = "/v3/apps/{guid}/builds"
	invalidDropletMsg                 = "Unable to assign current droplet. Ensure the droplet exists and belongs to this app."

	AppStartedState = "STARTED"
	AppStoppedState = "STOPPED"
	AppRevisionKey  = "korifi.cloudfoundry.org/app-rev"
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
	GetAppEnvVars(context.Context, authorization.Info, string) (repositories.AppEnvVarsRecord, error)
	PatchApp(context.Context, authorization.Info, repositories.PatchAppMessage) (repositories.AppRecord, error)
}

type App struct {
	serverURL        url.URL
	appRepo          CFAppRepository
	dropletRepo      CFDropletRepository
	processRepo      CFProcessRepository
	routeRepo        CFRouteRepository
	domainRepo       CFDomainRepository
	spaceRepo        CFSpaceRepository
	packageRepo      CFPackageRepository
	requestValidator RequestValidator
	buildRepo        CFBuildRepository
	processStats     ProcessStats
}

func NewApp(
	serverURL url.URL,
	appRepo CFAppRepository,
	dropletRepo CFDropletRepository,
	processRepo CFProcessRepository,
	routeRepo CFRouteRepository,
	domainRepo CFDomainRepository,
	spaceRepo CFSpaceRepository,
	packageRepo CFPackageRepository,
	buildRepo CFBuildRepository,
	processStats ProcessStats,
	requestValidator RequestValidator,
) *App {
	return &App{
		serverURL:        serverURL,
		appRepo:          appRepo,
		dropletRepo:      dropletRepo,
		processRepo:      processRepo,
		routeRepo:        routeRepo,
		domainRepo:       domainRepo,
		spaceRepo:        spaceRepo,
		packageRepo:      packageRepo,
		buildRepo:        buildRepo,
		processStats:     processStats,
		requestValidator: requestValidator,
	}
}

func (h *App) get(r *http.Request) (*routing.Response, error) {
	appGUID := routing.URLParam(r, "guid")
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
	if err := h.requestValidator.DecodeAndValidateJSONPayload(r, &payload); err != nil {
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

	err = h.processRepo.CreateProcess(r.Context(), authInfo, repositories.CreateProcessMessage{
		AppGUID:   appRecord.GUID,
		SpaceGUID: spaceGUID,
		Type:      korifiv1alpha1.ProcessTypeWeb,
	})
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "Failed to create web process", "App Name", payload.Name)
	}

	return routing.NewResponse(http.StatusCreated).WithBody(presenter.ForApp(appRecord, h.serverURL)), nil
}

func (h *App) list(r *http.Request) (*routing.Response, error) { //nolint:dupl
	authInfo, _ := authorization.InfoFromContext(r.Context())
	logger := logr.FromContextOrDiscard(r.Context()).WithName("handlers.app.list")

	appListFilter := new(payloads.AppList)
	err := h.requestValidator.DecodeAndValidateURLValues(r, appListFilter)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "Unable to decode request query parameters")
	}

	appList, err := h.appRepo.ListApps(r.Context(), authInfo, appListFilter.ToMessage())
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "Failed to fetch app(s) from Kubernetes")
	}

	h.sortList(appList, appListFilter.OrderBy)

	return routing.NewResponse(http.StatusOK).WithBody(presenter.ForList(presenter.ForApp, appList, h.serverURL, *r.URL)), nil
}

func timePtrAfter(t1, t2 *time.Time) bool {
	if t1 == nil || t2 == nil {
		return false
	}

	return (*t1).After(*t2)
}

func (h *App) sortList(appList []repositories.AppRecord, order string) {
	switch order {
	case "":
	case "created_at":
		sort.Slice(appList, func(i, j int) bool { return timePtrAfter(&appList[j].CreatedAt, &appList[i].CreatedAt) })
	case "-created_at":
		sort.Slice(appList, func(i, j int) bool { return timePtrAfter(&appList[i].CreatedAt, &appList[j].CreatedAt) })
	case "updated_at":
		sort.Slice(appList, func(i, j int) bool { return timePtrAfter(appList[j].UpdatedAt, appList[i].UpdatedAt) })
	case "-updated_at":
		sort.Slice(appList, func(i, j int) bool { return timePtrAfter(appList[i].UpdatedAt, appList[j].UpdatedAt) })
	case "name":
		sort.Slice(appList, func(i, j int) bool { return appList[i].Name < appList[j].Name })
	case "-name":
		sort.Slice(appList, func(i, j int) bool { return appList[i].Name > appList[j].Name })
	case "state":
		sort.Slice(appList, func(i, j int) bool { return appList[i].State < appList[j].State })
	case "-state":
		sort.Slice(appList, func(i, j int) bool { return appList[i].State > appList[j].State })
	}
}

func (h *App) setCurrentDroplet(r *http.Request) (*routing.Response, error) {
	authInfo, _ := authorization.InfoFromContext(r.Context())
	logger := logr.FromContextOrDiscard(r.Context()).WithName("handlers.app.set-current-droplet")
	appGUID := routing.URLParam(r, "guid")

	var payload payloads.AppSetCurrentDroplet
	if err := h.requestValidator.DecodeAndValidateJSONPayload(r, &payload); err != nil {
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
	appGUID := routing.URLParam(r, "guid")

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
	appGUID := routing.URLParam(r, "guid")

	app, err := h.appRepo.GetApp(r.Context(), authInfo, appGUID)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, apierrors.ForbiddenAsNotFound(err), "Failed to fetch app from Kubernetes", "AppGUID", appGUID)
	}

	app, err = h.startApp(r.Context(), authInfo, app)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "Failed to start app", "AppGUID", appGUID)
	}

	return routing.NewResponse(http.StatusOK).WithBody(presenter.ForApp(app, h.serverURL)), nil
}

func (h *App) startApp(ctx context.Context, authInfo authorization.Info, app repositories.AppRecord) (repositories.AppRecord, error) {
	if app.DropletGUID == "" {
		return repositories.AppRecord{}, apierrors.NewUnprocessableEntityError(errors.New("app droplet not set"), "Assign a droplet before starting this app.")
	}

	app, err := h.appRepo.SetAppDesiredState(ctx, authInfo, repositories.SetAppDesiredStateMessage{
		AppGUID:      app.GUID,
		SpaceGUID:    app.SpaceGUID,
		DesiredState: AppStartedState,
	})
	if err != nil {
		return repositories.AppRecord{}, fmt.Errorf("failed to update app desired state: %w", err)
	}

	return app, nil
}

func (h *App) stop(r *http.Request) (*routing.Response, error) {
	authInfo, _ := authorization.InfoFromContext(r.Context())
	logger := logr.FromContextOrDiscard(r.Context()).WithName("handlers.app.stop")
	appGUID := routing.URLParam(r, "guid")

	app, err := h.appRepo.GetApp(r.Context(), authInfo, appGUID)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, apierrors.ForbiddenAsNotFound(err), "Failed to fetch app from Kubernetes", "AppGUID", appGUID)
	}

	app, err = h.stopApp(r.Context(), authInfo, app)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "Failed to stop app", "AppGUID", appGUID)
	}

	return routing.NewResponse(http.StatusOK).WithBody(presenter.ForApp(app, h.serverURL)), nil
}

func (h *App) stopApp(ctx context.Context, authInfo authorization.Info, app repositories.AppRecord) (repositories.AppRecord, error) {
	app, err := h.appRepo.SetAppDesiredState(ctx, authInfo, repositories.SetAppDesiredStateMessage{
		AppGUID:      app.GUID,
		SpaceGUID:    app.SpaceGUID,
		DesiredState: AppStoppedState,
	})
	if err != nil {
		return repositories.AppRecord{}, fmt.Errorf("failed to update app desired state: %w", err)
	}

	return app, nil
}

func (h *App) getProcesses(r *http.Request) (*routing.Response, error) {
	authInfo, _ := authorization.InfoFromContext(r.Context())
	logger := logr.FromContextOrDiscard(r.Context()).WithName("handlers.app.get-processes")
	appGUID := routing.URLParam(r, "guid")

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
	appGUID := routing.URLParam(r, "guid")

	app, err := h.appRepo.GetApp(r.Context(), authInfo, appGUID)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, apierrors.ForbiddenAsNotFound(err), "Failed to fetch app from Kubernetes", "AppGUID", appGUID)
	}

	routes, err := h.lookupAppRouteAndDomainList(r.Context(), authInfo, app.GUID, app.SpaceGUID)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "Failed to fetch route or domains from Kubernetes")
	}

	return routing.NewResponse(http.StatusOK).WithBody(presenter.ForList(presenter.ForRoute, routes, h.serverURL, *r.URL)), nil
}

func (h *App) scaleProcess(r *http.Request) (*routing.Response, error) {
	authInfo, _ := authorization.InfoFromContext(r.Context())
	appGUID := routing.URLParam(r, "guid")
	processType := routing.URLParam(r, "processType")
	logger := logr.FromContextOrDiscard(r.Context()).
		WithName("handlers.app.scale-process").
		WithValues("appGUID", appGUID, "processType", processType)

	var payload payloads.ProcessScale
	if err := h.requestValidator.DecodeAndValidateJSONPayload(r, &payload); err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "failed to decode json payload")
	}

	app, err := h.appRepo.GetApp(r.Context(), authInfo, appGUID)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, apierrors.ForbiddenAsNotFound(err), "falied to get app")
	}

	appProcesses, err := h.processRepo.ListProcesses(r.Context(), authInfo, repositories.ListProcessesMessage{
		AppGUIDs:  []string{app.GUID},
		SpaceGUID: app.SpaceGUID,
	})
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "failed to list processes for app")
	}

	process, hasProcessType := findProcessType(appProcesses, processType)
	if !hasProcessType {
		return nil, apierrors.LogAndReturn(logger,
			apierrors.NewNotFoundError(nil, repositories.ProcessResourceType),
			"app does not have required process type",
		)
	}

	scaledProcessRecord, err := h.processRepo.ScaleProcess(r.Context(), authInfo, repositories.ScaleProcessMessage{
		GUID:               process.GUID,
		SpaceGUID:          app.SpaceGUID,
		ProcessScaleValues: payload.ToRecord(),
	})
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "Failed due to error from Kubernetes", "appGUID", appGUID)
	}

	return routing.NewResponse(http.StatusOK).WithBody(presenter.ForProcess(scaledProcessRecord, h.serverURL)), nil
}

func findProcessType(processes []repositories.ProcessRecord, processType string) (repositories.ProcessRecord, bool) {
	for _, proc := range processes {
		if proc.Type == processType {
			return proc, true
		}
	}

	return repositories.ProcessRecord{}, false
}

func (h *App) restart(r *http.Request) (*routing.Response, error) {
	authInfo, _ := authorization.InfoFromContext(r.Context())
	logger := logr.FromContextOrDiscard(r.Context()).WithName("handlers.app.restart")
	appGUID := routing.URLParam(r, "guid")

	app, err := h.appRepo.GetApp(r.Context(), authInfo, appGUID)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, apierrors.ForbiddenAsNotFound(err), "Failed to fetch app from Kubernetes", "AppGUID", appGUID)
	}

	app, err = h.stopApp(r.Context(), authInfo, app)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "Failed to stop app", "AppGUID", appGUID)
	}

	app, err = h.startApp(r.Context(), authInfo, app)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "Failed to start app", "AppGUID", appGUID)
	}

	return routing.NewResponse(http.StatusOK).WithBody(presenter.ForApp(app, h.serverURL)), nil
}

func (h *App) delete(r *http.Request) (*routing.Response, error) {
	authInfo, _ := authorization.InfoFromContext(r.Context())
	logger := logr.FromContextOrDiscard(r.Context()).WithName("handlers.app.delete")
	appGUID := routing.URLParam(r, "guid")

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
		routeRecords[i].Domain = domainRecord
	}

	return routeRecords, nil
}

func (h *App) updateEnvVars(r *http.Request) (*routing.Response, error) {
	authInfo, _ := authorization.InfoFromContext(r.Context())
	logger := logr.FromContextOrDiscard(r.Context()).WithName("handlers.app.update-env-vars")
	appGUID := routing.URLParam(r, "guid")

	var payload payloads.AppPatchEnvVars
	if err := h.requestValidator.DecodeAndValidateJSONPayload(r, &payload); err != nil {
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
	appGUID := routing.URLParam(r, "guid")

	appEnvRecord, err := h.appRepo.GetAppEnv(r.Context(), authInfo, appGUID)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "Failed to fetch app environment variables", "AppGUID", appGUID)
	}

	return routing.NewResponse(http.StatusOK).WithBody(presenter.ForAppEnv(appEnvRecord)), nil
}

func (h *App) getProcess(r *http.Request) (*routing.Response, error) {
	authInfo, _ := authorization.InfoFromContext(r.Context())
	logger := logr.FromContextOrDiscard(r.Context()).WithName("handlers.app.get-process")
	appGUID := routing.URLParam(r, "guid")
	processType := routing.URLParam(r, "type")

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

func (h *App) getPackages(r *http.Request) (*routing.Response, error) {
	authInfo, _ := authorization.InfoFromContext(r.Context())
	logger := logr.FromContextOrDiscard(r.Context()).WithName("handlers.app.get-packages")
	appGUID := routing.URLParam(r, "guid")

	_, err := h.appRepo.GetApp(r.Context(), authInfo, appGUID)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, apierrors.ForbiddenAsNotFound(err), "Failed to fetch app from Kubernetes", "AppGUID", appGUID)
	}

	fetchPackagesForAppMessage := repositories.ListPackagesMessage{
		AppGUIDs: []string{appGUID},
		States:   []string{},
	}

	packageList, err := h.packageRepo.ListPackages(r.Context(), authInfo, fetchPackagesForAppMessage)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "Failed to fetch app Package(s) from Kubernetes")
	}

	return routing.NewResponse(http.StatusOK).WithBody(presenter.ForList(presenter.ForPackage, packageList, h.serverURL, *r.URL)), nil
}

//nolint:dupl
func (h *App) update(r *http.Request) (*routing.Response, error) {
	authInfo, _ := authorization.InfoFromContext(r.Context())
	logger := logr.FromContextOrDiscard(r.Context()).WithName("handlers.app.update")
	appGUID := routing.URLParam(r, "guid")

	app, err := h.appRepo.GetApp(r.Context(), authInfo, appGUID)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, apierrors.ForbiddenAsNotFound(err), "Failed to fetch app from Kubernetes", "AppGUID", appGUID)
	}

	var payload payloads.AppPatch
	if err = h.requestValidator.DecodeAndValidateJSONPayload(r, &payload); err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "failed to decode payload")
	}

	app, err = h.appRepo.PatchApp(r.Context(), authInfo, payload.ToMessage(appGUID, app.SpaceGUID))
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "Failed to patch app", "AppGUID", appGUID)
	}
	return routing.NewResponse(http.StatusOK).WithBody(presenter.ForApp(app, h.serverURL)), nil
}

func (h *App) getSSHEnabled(r *http.Request) (*routing.Response, error) {
	return routing.NewResponse(http.StatusOK).WithBody(presenter.AppSSHEnabled{
		Enabled: false,
		Reason:  "Disabled globally",
	}), nil
}

func (h *App) updateAppFeature(r *http.Request) (*routing.Response, error) {
	featureName := routing.URLParam(r, "name")
	return routing.NewResponse(http.StatusOK).WithBody(map[string]any{
		"name":        featureName,
		"description": "Description of " + featureName,
		"enabled":     false,
	}), nil
}

func (h *App) getEnvVars(r *http.Request) (*routing.Response, error) {
	authInfo, _ := authorization.InfoFromContext(r.Context())
	logger := logr.FromContextOrDiscard(r.Context()).WithName("handlers.app.get-env-vars")
	appGUID := routing.URLParam(r, "guid")

	appEnvVarsRecord, err := h.appRepo.GetAppEnvVars(r.Context(), authInfo, appGUID)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "Error getting app environment variables")
	}

	return routing.NewResponse(http.StatusOK).WithBody(presenter.ForAppEnvVars(appEnvVarsRecord, h.serverURL)), nil
}

func (h *App) getAppBuilds(r *http.Request) (*routing.Response, error) {
	authInfo, _ := authorization.InfoFromContext(r.Context())
	logger := logr.FromContextOrDiscard(r.Context()).WithName("handlers.app.get.builds")

	appGUID := routing.URLParam(r, "guid")

	builds, err := h.buildRepo.ListAppBuilds(r.Context(), authInfo, repositories.ListAppBuildsMessage{AppGUID: appGUID})
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, apierrors.ForbiddenAsNotFound(err), "Failed to get build from Kubernetes", "AppGUID", appGUID)
	}

	return routing.NewResponse(http.StatusOK).WithBody(presenter.ForBuildList(builds, h.serverURL, *r.URL)), nil
}

func (h *App) getProcessStats(r *http.Request) (*routing.Response, error) {
	authInfo, _ := authorization.InfoFromContext(r.Context())
	logger := logr.FromContextOrDiscard(r.Context()).WithName("handlers.app.get-process-stats")
	appGUID := routing.URLParam(r, "guid")
	processType := routing.URLParam(r, "type")

	app, err := h.appRepo.GetApp(r.Context(), authInfo, appGUID)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, apierrors.ForbiddenAsNotFound(err), "Failed to fetch app from Kubernetes", "AppGUID", appGUID)
	}

	process, err := h.processRepo.GetProcessByAppTypeAndSpace(r.Context(), authInfo, appGUID, processType, app.SpaceGUID)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "Failed to fetch process from Kubernetes", "AppGUID", appGUID)
	}

	processGUID := process.GUID

	records, err := h.processStats.FetchStats(r.Context(), authInfo, processGUID)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, apierrors.ForbiddenAsNotFound(err), "Failed to get process stats from Kubernetes", "ProcessGUID", processGUID)
	}

	return routing.NewResponse(http.StatusOK).WithBody(presenter.ForProcessStats(records)), nil
}

func (h *App) UnauthenticatedRoutes() []routing.Route {
	return nil
}

func (h *App) AuthenticatedRoutes() []routing.Route {
	return []routing.Route{
		{Method: "GET", Pattern: AppPath, Handler: h.get},
		{Method: "GET", Pattern: AppsPath, Handler: h.list},
		{Method: "POST", Pattern: AppsPath, Handler: h.create},
		{Method: "PATCH", Pattern: AppCurrentDropletRelationshipPath, Handler: h.setCurrentDroplet},
		{Method: "GET", Pattern: AppCurrentDropletPath, Handler: h.getCurrentDroplet},
		{Method: "POST", Pattern: AppStartPath, Handler: h.start},
		{Method: "POST", Pattern: AppStopPath, Handler: h.stop},
		{Method: "POST", Pattern: AppRestartPath, Handler: h.restart},
		{Method: "POST", Pattern: AppProcessScalePath, Handler: h.scaleProcess},
		{Method: "GET", Pattern: AppProcessesPath, Handler: h.getProcesses},
		{Method: "GET", Pattern: AppProcessByTypePath, Handler: h.getProcess},
		{Method: "GET", Pattern: AppRoutesPath, Handler: h.getRoutes},
		{Method: "DELETE", Pattern: AppPath, Handler: h.delete},
		{Method: "PATCH", Pattern: AppEnvVarsPath, Handler: h.updateEnvVars},
		{Method: "GET", Pattern: AppEnvVarsPath, Handler: h.getEnvVars},
		{Method: "GET", Pattern: AppEnvPath, Handler: h.getEnvironment},
		{Method: "GET", Pattern: AppPackagesPath, Handler: h.getPackages},
		{Method: "PATCH", Pattern: AppPath, Handler: h.update},
		{Method: "GET", Pattern: AppSSHEnabledPath, Handler: h.getSSHEnabled},
		{Method: "PATCH", Pattern: AppFeaturePath, Handler: h.updateAppFeature},
		{Method: "GET", Pattern: AppBuildsPath, Handler: h.getAppBuilds},
		{Method: "GET", Pattern: AppProcessStatsByTypePath, Handler: h.getProcessStats},
	}
}
