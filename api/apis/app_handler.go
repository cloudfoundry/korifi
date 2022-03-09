package apis

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/gorilla/schema"

	"code.cloudfoundry.org/cf-k8s-controllers/controllers/webhooks"

	"code.cloudfoundry.org/cf-k8s-controllers/api/apierrors"
	"code.cloudfoundry.org/cf-k8s-controllers/api/authorization"
	"code.cloudfoundry.org/cf-k8s-controllers/api/payloads"
	"code.cloudfoundry.org/cf-k8s-controllers/api/presenter"
	"code.cloudfoundry.org/cf-k8s-controllers/api/repositories"

	"github.com/go-logr/logr"
	"github.com/gorilla/mux"
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
	GetAppByNameAndSpace(context.Context, authorization.Info, string, string) (repositories.AppRecord, error)
	ListApps(context.Context, authorization.Info, repositories.ListAppsMessage) ([]repositories.AppRecord, error)
	CreateOrPatchAppEnvVars(context.Context, authorization.Info, repositories.CreateOrPatchAppEnvVarsMessage) (repositories.AppEnvVarsRecord, error)
	PatchAppEnvVars(context.Context, authorization.Info, repositories.PatchAppEnvVarsMessage) (repositories.AppEnvVarsRecord, error)
	CreateApp(context.Context, authorization.Info, repositories.CreateAppMessage) (repositories.AppRecord, error)
	SetCurrentDroplet(context.Context, authorization.Info, repositories.SetCurrentDropletMessage) (repositories.CurrentDropletRecord, error)
	SetAppDesiredState(context.Context, authorization.Info, repositories.SetAppDesiredStateMessage) (repositories.AppRecord, error)
	DeleteApp(context.Context, authorization.Info, repositories.DeleteAppMessage) error
	GetAppEnv(context.Context, authorization.Info, string) (map[string]string, error)
}

//counterfeiter:generate -o fake -fake-name ScaleAppProcess . ScaleAppProcess
type ScaleAppProcess func(ctx context.Context, authInfo authorization.Info, appGUID string, processType string, scale repositories.ProcessScaleValues) (repositories.ProcessRecord, error)

type AppHandler struct {
	logger           logr.Logger
	serverURL        url.URL
	appRepo          CFAppRepository
	dropletRepo      CFDropletRepository
	processRepo      CFProcessRepository
	routeRepo        CFRouteRepository
	domainRepo       CFDomainRepository
	podRepo          PodRepository
	spaceRepo        SpaceRepository
	scaleAppProcess  ScaleAppProcess
	decoderValidator *DecoderValidator
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
	spaceRepo SpaceRepository,
	scaleAppProcessFunc ScaleAppProcess,
	decoderValidator *DecoderValidator,
) *AppHandler {
	return &AppHandler{
		logger:           logger,
		serverURL:        serverURL,
		appRepo:          appRepo,
		dropletRepo:      dropletRepo,
		processRepo:      processRepo,
		routeRepo:        routeRepo,
		domainRepo:       domainRepo,
		decoderValidator: decoderValidator,
		podRepo:          podRepo,
		spaceRepo:        spaceRepo,
		scaleAppProcess:  scaleAppProcessFunc,
	}
}

func (h *AppHandler) appGetHandler(authInfo authorization.Info, r *http.Request) (*HandlerResponse, error) {
	ctx := r.Context()

	vars := mux.Vars(r)
	appGUID := vars["guid"]

	app, err := h.appRepo.GetApp(ctx, authInfo, appGUID)
	if err != nil {
		return nil, h.handleGetAppErr(err, appGUID)
	}
	return NewHandlerResponse(http.StatusOK).WithBody(presenter.ForApp(app, h.serverURL)), nil
}

func (h *AppHandler) appCreateHandler(authInfo authorization.Info, r *http.Request) (*HandlerResponse, error) {
	ctx := r.Context()

	var payload payloads.AppCreate
	if err := h.decoderValidator.DecodeAndValidateJSONPayload(r, &payload); err != nil {
		return nil, err
	}

	spaceGUID := payload.Relationships.Space.Data.GUID
	_, err := h.spaceRepo.GetSpace(ctx, authInfo, spaceGUID)
	if err != nil {
		switch err.(type) {
		case repositories.NotFoundError:
			h.logger.Info("Namespace not found", "Namespace GUID", spaceGUID)
			return nil, apierrors.NewUnprocessableEntityError(err, "Invalid space. Ensure that the space exists and you have access to it.")
		default:
			h.logger.Error(err, "Failed to fetch space from Kubernetes", "spaceGUID", spaceGUID)
			return nil, err
		}
	}

	appRecord, err := h.appRepo.CreateApp(ctx, authInfo, payload.ToAppCreateMessage())
	if err != nil {
		if webhooks.HasErrorCode(err, webhooks.DuplicateAppError) {
			errorDetail := fmt.Sprintf("App with the name '%s' already exists.", payload.Name)
			h.logger.Error(err, errorDetail, "App Name", payload.Name)
			return nil, apierrors.NewUniquenessError(err, errorDetail)
		}

		if k8serrors.IsForbidden(err) {
			h.logger.Error(err, "Not authorized to create app", "App Name", payload.Name)
			return nil, apierrors.NewForbiddenError(err, repositories.AppResourceType)
		}

		h.logger.Error(err, "Failed to create app", "App Name", payload.Name)
		return nil, err
	}

	return NewHandlerResponse(http.StatusCreated).WithBody(presenter.ForApp(appRecord, h.serverURL)), nil
}

func (h *AppHandler) appListHandler(authInfo authorization.Info, r *http.Request) (*HandlerResponse, error) { //nolint:dupl
	ctx := r.Context()

	if err := r.ParseForm(); err != nil {
		h.logger.Error(err, "Unable to parse request query parameters")
		return nil, err
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
					return nil, apierrors.NewUnknownKeyError(err, appListFilter.SupportedFilterKeys())
				}
			}

			h.logger.Error(err, "Unable to decode request query parameters")
			return nil, err
		default:
			h.logger.Error(err, "Unable to decode request query parameters")
			return nil, err
		}
	}

	appList, err := h.appRepo.ListApps(ctx, authInfo, appListFilter.ToMessage())
	if err != nil {
		h.logger.Error(err, "Failed to fetch app(s) from Kubernetes")
		return nil, err
	}

	return NewHandlerResponse(http.StatusOK).WithBody(presenter.ForAppList(appList, h.serverURL, *r.URL)), nil
}

func (h *AppHandler) appSetCurrentDropletHandler(authInfo authorization.Info, r *http.Request) (*HandlerResponse, error) {
	ctx := r.Context()
	vars := mux.Vars(r)
	appGUID := vars["guid"]

	var payload payloads.AppSetCurrentDroplet
	if err := h.decoderValidator.DecodeAndValidateJSONPayload(r, &payload); err != nil {
		return nil, err
	}

	app, err := h.appRepo.GetApp(ctx, authInfo, appGUID)
	if err != nil {
		return nil, h.handleGetAppErr(err, appGUID)
	}

	dropletGUID := payload.Data.GUID
	droplet, err := h.dropletRepo.GetDroplet(ctx, authInfo, dropletGUID)
	if err != nil {
		switch err.(type) {
		case repositories.NotFoundError:
			h.logger.Error(err, "Droplet not found", "dropletGUID", dropletGUID)
			return nil, apierrors.NewUnprocessableEntityError(err, invalidDropletMsg)
		case repositories.ForbiddenError:
			h.logger.Error(err, "Droplet not authorized for user", "dropletGUID", dropletGUID)
			return nil, apierrors.NewUnprocessableEntityError(err, invalidDropletMsg)
		default:
			h.logger.Error(err, "Error fetching droplet")
			return nil, err
		}
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
		h.logger.Error(err, "Error setting current droplet")
		switch {
		case errors.As(err, &authorization.NotAuthenticatedError{}):
			return nil, apierrors.NewNotAuthenticatedError(err)
		case errors.As(err, &authorization.InvalidAuthError{}):
			return nil, apierrors.NewInvalidAuthError(err)
		case errors.As(err, &repositories.ForbiddenError{}):
			return nil, apierrors.NewForbiddenError(err, repositories.AppResourceType)
		default:
			return nil, err
		}
	}

	return NewHandlerResponse(http.StatusOK).WithBody(presenter.ForCurrentDroplet(currentDroplet, h.serverURL)), nil
}

func (h *AppHandler) appGetCurrentDropletHandler(authInfo authorization.Info, r *http.Request) (*HandlerResponse, error) {
	ctx := r.Context()
	vars := mux.Vars(r)
	appGUID := vars["guid"]

	app, err := h.appRepo.GetApp(ctx, authInfo, appGUID)
	if err != nil {
		return nil, h.handleGetAppErr(err, appGUID)
	}

	if app.DropletGUID == "" {
		h.logger.Info("App does not have a current droplet assigned", "appGUID", app.GUID)
		return nil, apierrors.NewNotFoundError(err, repositories.DropletResourceType)
	}

	droplet, err := h.dropletRepo.GetDroplet(ctx, authInfo, app.DropletGUID)
	if err != nil {
		switch err.(type) {
		case repositories.NotFoundError:
			h.logger.Info("Droplet not found", "dropletGUID", app.DropletGUID)
			return nil, apierrors.NewNotFoundError(err, repositories.DropletResourceType)
		case repositories.ForbiddenError:
			h.logger.Info("Droplet not authorized to user", "dropletGUID", app.DropletGUID)
			return nil, apierrors.NewNotFoundError(err, repositories.DropletResourceType)
		default:
			h.logger.Error(err, "Failed to fetch droplet from Kubernetes", "dropletGUID", app.DropletGUID)
			return nil, err
		}
	}

	return NewHandlerResponse(http.StatusOK).WithBody(presenter.ForDroplet(droplet, h.serverURL)), nil
}

func (h *AppHandler) appStartHandler(authInfo authorization.Info, r *http.Request) (*HandlerResponse, error) {
	ctx := r.Context()

	vars := mux.Vars(r)
	appGUID := vars["guid"]

	app, err := h.appRepo.GetApp(ctx, authInfo, appGUID)
	if err != nil {
		return nil, h.handleGetAppErr(err, appGUID)
	}
	if app.DropletGUID == "" {
		h.logger.Info("App droplet not set before start", "AppGUID", appGUID)
		return nil, apierrors.NewUnprocessableEntityError(err, "Assign a droplet before starting this app.")
	}

	app, err = h.appRepo.SetAppDesiredState(ctx, authInfo, repositories.SetAppDesiredStateMessage{
		AppGUID:      app.GUID,
		SpaceGUID:    app.SpaceGUID,
		DesiredState: AppStartedState,
	})
	if err != nil {
		return nil, h.handleUpdateAppErr(err, appGUID)
	}

	return NewHandlerResponse(http.StatusOK).WithBody(presenter.ForApp(app, h.serverURL)), nil
}

func (h *AppHandler) appStopHandler(authInfo authorization.Info, r *http.Request) (*HandlerResponse, error) {
	ctx := r.Context()

	vars := mux.Vars(r)
	appGUID := vars["guid"]

	app, err := h.appRepo.GetApp(ctx, authInfo, appGUID)
	if err != nil {
		return nil, h.handleGetAppErr(err, appGUID)
	}

	app, err = h.appRepo.SetAppDesiredState(ctx, authInfo, repositories.SetAppDesiredStateMessage{
		AppGUID:      app.GUID,
		SpaceGUID:    app.SpaceGUID,
		DesiredState: AppStoppedState,
	})
	if err != nil {
		if errors.As(err, &repositories.ForbiddenError{}) {
			h.logger.Info("failed to stop app", "AppGUID", appGUID, "error", err)
			return nil, apierrors.NewForbiddenError(err, repositories.AppResourceType)
		}

		h.logger.Error(err, "Failed to update app in Kubernetes", "AppGUID", appGUID)
		return nil, err
	}

	return NewHandlerResponse(http.StatusOK).WithBody(presenter.ForApp(app, h.serverURL)), nil
}

func (h *AppHandler) getProcessesForAppHandler(authInfo authorization.Info, r *http.Request) (*HandlerResponse, error) {
	ctx := r.Context()

	vars := mux.Vars(r)
	appGUID := vars["guid"]

	app, err := h.appRepo.GetApp(ctx, authInfo, appGUID)
	if err != nil {
		return nil, h.handleGetAppErr(err, appGUID)
	}

	fetchProcessesForAppMessage := repositories.ListProcessesMessage{
		AppGUID:   []string{appGUID},
		SpaceGUID: app.SpaceGUID,
	}

	processList, err := h.processRepo.ListProcesses(ctx, authInfo, fetchProcessesForAppMessage)
	if err != nil {
		h.logger.Error(err, "Failed to fetch app Process(es) from Kubernetes")
		return nil, err
	}

	return NewHandlerResponse(http.StatusOK).WithBody(presenter.ForProcessList(processList, h.serverURL, *r.URL)), nil
}

func (h *AppHandler) getRoutesForAppHandler(authInfo authorization.Info, r *http.Request) (*HandlerResponse, error) {
	ctx := r.Context()

	vars := mux.Vars(r)
	appGUID := vars["guid"]

	app, err := h.appRepo.GetApp(ctx, authInfo, appGUID)
	if err != nil {
		return nil, h.handleGetAppErr(err, appGUID)
	}

	routes, err := h.lookupAppRouteAndDomainList(ctx, authInfo, app.GUID, app.SpaceGUID)
	if err != nil {
		h.logger.Error(err, "Failed to fetch route or domains from Kubernetes")
		return nil, err
	}

	return NewHandlerResponse(http.StatusOK).WithBody(presenter.ForRouteList(routes, h.serverURL, *r.URL)), nil
}

func (h *AppHandler) appScaleProcessHandler(authInfo authorization.Info, r *http.Request) (*HandlerResponse, error) {
	ctx := r.Context()

	vars := mux.Vars(r)
	appGUID := vars["guid"]
	processType := vars["processType"]

	var payload payloads.ProcessScale
	if err := h.decoderValidator.DecodeAndValidateJSONPayload(r, &payload); err != nil {
		return nil, err
	}

	processRecord, err := h.scaleAppProcess(ctx, authInfo, appGUID, processType, payload.ToRecord())
	if err != nil {
		switch errType := err.(type) {
		case repositories.NotFoundError:
			resourceType := errType.ResourceType()
			h.logger.Info(fmt.Sprintf("%s not found", resourceType), "appGUID", appGUID)
			return nil, apierrors.NewNotFoundError(err, resourceType)
		default:
			h.logger.Error(err, "Failed due to error from Kubernetes", "appGUID", appGUID)
			return nil, err
		}
	}

	return NewHandlerResponse(http.StatusOK).WithBody(presenter.ForProcess(processRecord, h.serverURL)), nil
}

func (h *AppHandler) appRestartHandler(authInfo authorization.Info, r *http.Request) (*HandlerResponse, error) {
	ctx := r.Context()

	vars := mux.Vars(r)
	appGUID := vars["guid"]

	app, err := h.appRepo.GetApp(ctx, authInfo, appGUID)
	if err != nil {
		return nil, h.handleGetAppErr(err, appGUID)
	}

	if app.DropletGUID == "" {
		h.logger.Info("App droplet not set before start", "AppGUID", appGUID)
		return nil, apierrors.NewUnprocessableEntityError(fmt.Errorf("app %s has no droplet set", app.GUID), "Assign a droplet before starting this app.")
	}

	if app.State == repositories.StartedState {
		app, err = h.appRepo.SetAppDesiredState(ctx, authInfo, repositories.SetAppDesiredStateMessage{
			AppGUID:      app.GUID,
			SpaceGUID:    app.SpaceGUID,
			DesiredState: AppStoppedState,
		})
		if err != nil {
			switch err.(type) {
			case repositories.ForbiddenError:
				h.logger.Info("failed to stop app", "AppGUID", appGUID)
				return nil, apierrors.NewForbiddenError(err, repositories.AppResourceType)
			default:
				h.logger.Error(err, "Failed to update app in Kubernetes", "AppGUID", appGUID)
				return nil, err
			}
		}
	}

	app, err = h.appRepo.SetAppDesiredState(ctx, authInfo, repositories.SetAppDesiredStateMessage{
		AppGUID:      app.GUID,
		SpaceGUID:    app.SpaceGUID,
		DesiredState: AppStartedState,
	})
	if err != nil {
		switch err.(type) {
		case repositories.ForbiddenError:
			h.logger.Info("failed to start app", "AppGUID", appGUID)
			return nil, apierrors.NewForbiddenError(err, repositories.AppResourceType)
		default:
			h.logger.Error(err, "Failed to update app in Kubernetes", "AppGUID", appGUID)
			return nil, err
		}
	}

	return NewHandlerResponse(http.StatusOK).WithBody(presenter.ForApp(app, h.serverURL)), nil
}

func (h *AppHandler) appDeleteHandler(authInfo authorization.Info, r *http.Request) (*HandlerResponse, error) {
	ctx := r.Context()
	vars := mux.Vars(r)
	appGUID := vars["guid"]

	app, err := h.appRepo.GetApp(ctx, authInfo, appGUID)
	if err != nil {
		return nil, h.handleGetAppErr(err, appGUID)
	}

	err = h.appRepo.DeleteApp(ctx, authInfo, repositories.DeleteAppMessage{
		AppGUID:   appGUID,
		SpaceGUID: app.SpaceGUID,
	})
	if err != nil {
		h.logger.Error(err, "Failed to delete app", "AppGUID", appGUID)
		return nil, err
	}

	return NewHandlerResponse(http.StatusAccepted).WithHeader("Location", fmt.Sprintf("%s/v3/jobs/app.delete-%s", h.serverURL.String(), appGUID)), nil
}

func (h *AppHandler) lookupAppRouteAndDomainList(ctx context.Context, authInfo authorization.Info, appGUID, spaceGUID string) ([]repositories.RouteRecord, error) {
	routeRecords, err := h.routeRepo.ListRoutesForApp(ctx, authInfo, appGUID, spaceGUID)
	if err != nil {
		return []repositories.RouteRecord{}, err
	}

	return getDomainsForRoutes(ctx, h.domainRepo, authInfo, routeRecords)
}

func (h *AppHandler) appPatchEnvVarsHandler(authInfo authorization.Info, r *http.Request) (*HandlerResponse, error) {
	ctx := r.Context()
	vars := mux.Vars(r)
	appGUID := vars["guid"]

	var payload payloads.AppPatchEnvVars
	if err := h.decoderValidator.DecodeAndValidateJSONPayload(r, &payload); err != nil {
		return nil, err
	}

	app, err := h.appRepo.GetApp(ctx, authInfo, appGUID)
	if err != nil {
		return nil, h.handleGetAppErr(err, appGUID)
	}

	envVarsRecord, err := h.appRepo.PatchAppEnvVars(ctx, authInfo, payload.ToMessage(appGUID, app.SpaceGUID))
	if err != nil {
		h.logger.Error(err, "Error updating app environment variables")
		return nil, err
	}

	return NewHandlerResponse(http.StatusOK).WithBody(presenter.ForAppEnvVars(envVarsRecord, h.serverURL)), nil
}

func (h *AppHandler) appGetEnvHandler(authInfo authorization.Info, r *http.Request) (*HandlerResponse, error) {
	vars := mux.Vars(r)
	appGUID := vars["guid"]

	envVars, err := h.appRepo.GetAppEnv(r.Context(), authInfo, appGUID)
	if err != nil {
		switch err.(type) {
		case repositories.NotFoundError:
			h.logger.Error(err, "App not found", "AppGUID", appGUID)
			return nil, apierrors.NewNotFoundError(err, repositories.AppResourceType)
		case repositories.ForbiddenError:
			h.logger.Error(err, "not allowed to fetch App env")
			return nil, apierrors.NewForbiddenError(err, repositories.AppResourceType)
		default:
			h.logger.Error(err, "Failed to fetch app environment variables", "AppGUID", appGUID)
			return nil, err
		}
	}

	return NewHandlerResponse(http.StatusOK).WithBody(presenter.ForAppEnv(envVars)), nil
}

func (h *AppHandler) getProcessByTypeForAppHander(authInfo authorization.Info, r *http.Request) (*HandlerResponse, error) {
	ctx := r.Context()

	vars := mux.Vars(r)
	appGUID := vars["guid"]
	processType := vars["type"]

	app, err := h.appRepo.GetApp(ctx, authInfo, appGUID)
	if err != nil {
		return nil, h.handleGetAppErr(err, appGUID)
	}

	process, err := h.processRepo.GetProcessByAppTypeAndSpace(ctx, authInfo, appGUID, processType, app.SpaceGUID)
	if err != nil {
		switch err.(type) {
		case repositories.NotFoundError:
			h.logger.Info("Process not found", "AppGUID", appGUID)
			return nil, apierrors.NewNotFoundError(err, repositories.ProcessResourceType)
		case repositories.ForbiddenError:
			h.logger.Info("Process forbidden", "AppGUID", appGUID)
			return nil, apierrors.NewForbiddenError(err, repositories.ProcessResourceType)
		default:
			h.logger.Error(err, "Failed to fetch process from Kubernetes", "AppGUID", appGUID)
			return nil, err
		}
	}

	return NewHandlerResponse(http.StatusOK).WithBody(presenter.ForProcess(process, h.serverURL)), nil
}

func (h *AppHandler) handleGetAppErr(err error, appGUID string) error {
	switch err.(type) {
	case repositories.NotFoundError:
		h.logger.Info("App not found", "AppGUID", appGUID)
		return apierrors.NewNotFoundError(err, repositories.AppResourceType)
	case repositories.ForbiddenError:
		h.logger.Info("App forbidden", "AppGUID", appGUID)
		return apierrors.NewNotFoundError(err, repositories.AppResourceType)
	default:
		h.logger.Error(err, "Failed to fetch app from Kubernetes", "AppGUID", appGUID)
		return err
	}
}

func (h *AppHandler) handleUpdateAppErr(err error, appGUID string) error {
	switch err.(type) {
	case repositories.NotFoundError:
		h.logger.Info("App not found", "AppGUID", appGUID)
		return apierrors.NewNotFoundError(err, repositories.AppResourceType)
	case repositories.ForbiddenError:
		h.logger.Info("App forbidden", "AppGUID", appGUID)
		return apierrors.NewForbiddenError(err, repositories.AppResourceType)
	default:
		h.logger.Error(err, "Failed to update app in Kubernetes", "AppGUID", appGUID)
		return err
	}
}

func (h *AppHandler) RegisterRoutes(router *mux.Router) {
	w := NewAuthAwareHandlerFuncWrapper(h.logger)
	router.Path(AppPath).Methods("GET").HandlerFunc(w.Wrap(h.appGetHandler))
	router.Path(AppsPath).Methods("GET").HandlerFunc(w.Wrap(h.appListHandler))
	router.Path(AppsPath).Methods("POST").HandlerFunc(w.Wrap(h.appCreateHandler))
	router.Path(AppCurrentDropletRelationshipPath).Methods("PATCH").HandlerFunc(w.Wrap(h.appSetCurrentDropletHandler))
	router.Path(AppCurrentDropletPath).Methods("GET").HandlerFunc(w.Wrap(h.appGetCurrentDropletHandler))
	router.Path(AppStartPath).Methods("POST").HandlerFunc(w.Wrap(h.appStartHandler))
	router.Path(AppStopPath).Methods("POST").HandlerFunc(w.Wrap(h.appStopHandler))
	router.Path(AppRestartPath).Methods("POST").HandlerFunc(w.Wrap(h.appRestartHandler))
	router.Path(AppProcessScalePath).Methods("POST").HandlerFunc(w.Wrap(h.appScaleProcessHandler))
	router.Path(AppProcessesPath).Methods("GET").HandlerFunc(w.Wrap(h.getProcessesForAppHandler))
	router.Path(AppProcessByTypePath).Methods("GET").HandlerFunc(w.Wrap(h.getProcessByTypeForAppHander))
	router.Path(AppRoutesPath).Methods("GET").HandlerFunc(w.Wrap(h.getRoutesForAppHandler))
	router.Path(AppPath).Methods("DELETE").HandlerFunc(w.Wrap(h.appDeleteHandler))
	router.Path(AppEnvVarsPath).Methods("PATCH").HandlerFunc(w.Wrap(h.appPatchEnvVarsHandler))
	router.Path(AppEnvPath).Methods("GET").HandlerFunc(w.Wrap(h.appGetEnvHandler))
}
