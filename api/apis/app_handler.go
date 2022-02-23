package apis

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/gorilla/schema"

	"code.cloudfoundry.org/cf-k8s-controllers/controllers/webhooks/workloads"

	"code.cloudfoundry.org/cf-k8s-controllers/api/apierr"
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
	AppGetProcessByTypeEndpoint  = "/v3/apps/{guid}/processes/{type}"
	AppProcessScaleEndpoint      = "/v3/apps/{guid}/processes/{processType}/actions/scale"
	AppGetRoutesEndpoint         = "/v3/apps/{guid}/routes"
	AppStartEndpoint             = "/v3/apps/{guid}/actions/start"
	AppStopEndpoint              = "/v3/apps/{guid}/actions/stop"
	AppRestartEndpoint           = "/v3/apps/{guid}/actions/restart"
	AppDeleteEndpoint            = "/v3/apps/{guid}"
	AppPatchEnvVarsEndpoint      = "/v3/apps/{guid}/environment_variables"
	AppGetEnvEndpoint            = "/v3/apps/{guid}/env"
	invalidDropletMsg            = "Unable to assign current droplet. Ensure the droplet exists and belongs to this app."

	AppStartedState = "STARTED"
	AppStoppedState = "STOPPED"
)

//counterfeiter:generate -o fake -fake-name CFAppRepository . CFAppRepository
type CFAppRepository interface {
	GetApp(context.Context, authorization.Info, string) (repositories.AppRecord, error)
	GetApp__NewStyle(context.Context, authorization.Info, string) (repositories.AppRecord, error)
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

func (h *AppHandler) appGetHandler(authInfo authorization.Info, w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	w.Header().Set("Content-Type", "application/json")

	vars := mux.Vars(r)
	appGUID := vars["guid"]

	app, err := h.appRepo.GetApp__NewStyle(ctx, authInfo, appGUID)
	if err != nil {
		writeErrorResponse(w, presenter.ForReadError(err))
		return
	}

	writeResponse(w, http.StatusOK, presenter.ForApp(app, h.serverURL))
}

func writeErrorResponse(w http.ResponseWriter, errResponse presenter.ErrorResponse) {
	writeResponse(w, errResponse.StatusCode, errResponse.Body)
}

func (h *AppHandler) appCreateHandler(authInfo authorization.Info, w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	w.Header().Set("Content-Type", "application/json")

	var payload payloads.AppCreate
	rme := h.decoderValidator.DecodeAndValidateJSONPayload(r, &payload)
	if rme != nil {
		writeRequestMalformedErrorResponse(w, rme)
		return
	}

	spaceGUID := payload.Relationships.Space.Data.GUID
	_, err := h.spaceRepo.GetSpace(ctx, authInfo, spaceGUID)
	if err != nil {
		switch err.(type) {
		case repositories.NotFoundError:
			h.logger.Info("Namespace not found", "Namespace GUID", spaceGUID)
			writeUnprocessableEntityError(w, "Invalid space. Ensure that the space exists and you have access to it.")
			return
		default:
			h.logger.Error(err, "Failed to fetch space from Kubernetes", "spaceGUID", spaceGUID)
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

		if k8serrors.IsForbidden(err) {
			h.logger.Error(err, "Not authorized to create app", "App Name", payload.Name)
			writeNotAuthorizedErrorResponse(w)
			return
		}

		h.logger.Error(err, "Failed to create app", "App Name", payload.Name)
		writeUnknownErrorResponse(w)
		return
	}

	writeResponse(w, http.StatusCreated, presenter.ForApp(appRecord, h.serverURL))
}

func (h *AppHandler) appListHandler(authInfo authorization.Info, w http.ResponseWriter, r *http.Request) {
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

	appList, err := h.appRepo.ListApps(ctx, authInfo, appListFilter.ToMessage())
	if err != nil {
		h.logger.Error(err, "Failed to fetch app(s) from Kubernetes")
		writeUnknownErrorResponse(w)
		return
	}

	writeResponse(w, http.StatusOK, presenter.ForAppList(appList, h.serverURL, *r.URL))
}

func (h *AppHandler) appSetCurrentDropletHandler(authInfo authorization.Info, w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	w.Header().Set("Content-Type", "application/json")
	vars := mux.Vars(r)
	appGUID := vars["guid"]

	var payload payloads.AppSetCurrentDroplet
	err := h.decoderValidator.DecodeAndValidateJSONPayload__NewStyle(r, &payload)
	if err != nil {
		writeErrorResponse(w, presenter.ForError(err))
		return
	}

	app, err := h.appRepo.GetApp__NewStyle(ctx, authInfo, appGUID)
	if err != nil {
		writeErrorResponse(w, presenter.ForReadError(err))
		return
	}

	dropletGUID := payload.Data.GUID
	droplet, err := h.dropletRepo.GetDroplet__NewStyle(ctx, authInfo, dropletGUID)
	if err != nil {
		h.logger.Info("GetDroplet failed", "dropletGUID", dropletGUID, "error", err)

		var apiErr apierr.ApiError
		if errors.As(err, &apiErr) {
			writeErrorResponse(w, presenter.ForError(apierr.NewUnprocessableEntityError(err, invalidDropletMsg)))
			return
		}

		writeErrorResponse(w, presenter.ForError(err))
		return
	}

	if droplet.AppGUID != appGUID {
		writeErrorResponse(w, presenter.ForError(apierr.NewUnprocessableEntityError(nil, invalidDropletMsg)))
		return
	}

	currentDroplet, err := h.appRepo.SetCurrentDroplet(ctx, authInfo, repositories.SetCurrentDropletMessage{
		AppGUID:     appGUID,
		DropletGUID: dropletGUID,
		SpaceGUID:   app.SpaceGUID,
	})
	if err != nil {
		h.logger.Error(err, "Error setting current droplet")
		writeErrorResponse(w, presenter.ForError(err))
		return
	}

	writeResponse(w, http.StatusOK, presenter.ForCurrentDroplet(currentDroplet, h.serverURL))
}

func (h *AppHandler) appGetCurrentDropletHandler(authInfo authorization.Info, w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	w.Header().Set("Content-Type", "application/json")
	vars := mux.Vars(r)
	appGUID := vars["guid"]

	app, err := h.appRepo.GetApp(ctx, authInfo, appGUID)
	if err != nil {
		h.handleGetAppErr(err, w, appGUID)
		return
	}

	if app.DropletGUID == "" {
		h.logger.Info("App does not have a current droplet assigned", "appGUID", app.GUID)
		writeNotFoundErrorResponse(w, "Droplet")
		return
	}

	droplet, err := h.dropletRepo.GetDroplet(ctx, authInfo, app.DropletGUID)
	if err != nil {
		switch err.(type) {
		case repositories.NotFoundError:
			h.logger.Info("Droplet not found", "dropletGUID", app.DropletGUID)
			writeNotFoundErrorResponse(w, "Droplet")
			return
		case repositories.ForbiddenError:
			h.logger.Info("Droplet not authorized to user", "dropletGUID", app.DropletGUID)
			writeNotFoundErrorResponse(w, "Droplet")
			return
		default:
			h.logger.Error(err, "Failed to fetch droplet from Kubernetes", "dropletGUID", app.DropletGUID)
			writeUnknownErrorResponse(w)
			return
		}
	}

	writeResponse(w, http.StatusOK, presenter.ForDroplet(droplet, h.serverURL))
}

func (h *AppHandler) appStartHandler(authInfo authorization.Info, w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	w.Header().Set("Content-Type", "application/json")

	vars := mux.Vars(r)
	appGUID := vars["guid"]

	app, err := h.appRepo.GetApp(ctx, authInfo, appGUID)
	if err != nil {
		h.handleGetAppErr(err, w, appGUID)
		return
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

	writeResponse(w, http.StatusOK, presenter.ForApp(app, h.serverURL))
}

func (h *AppHandler) appStopHandler(authInfo authorization.Info, w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	w.Header().Set("Content-Type", "application/json")

	vars := mux.Vars(r)
	appGUID := vars["guid"]

	app, err := h.appRepo.GetApp(ctx, authInfo, appGUID)
	if err != nil {
		h.handleGetAppErr(err, w, appGUID)
		return
	}

	app, err = h.appRepo.SetAppDesiredState(ctx, authInfo, repositories.SetAppDesiredStateMessage{
		AppGUID:      app.GUID,
		SpaceGUID:    app.SpaceGUID,
		DesiredState: AppStoppedState,
	})
	if err != nil {
		if errors.As(err, &repositories.ForbiddenError{}) {
			h.logger.Info("failed to stop app", "AppGUID", appGUID, "error", err)
			writeNotAuthorizedErrorResponse(w)
			return
		}

		h.logger.Error(err, "Failed to update app in Kubernetes", "AppGUID", appGUID)
		writeUnknownErrorResponse(w)
		return
	}

	writeResponse(w, http.StatusOK, presenter.ForApp(app, h.serverURL))
}

func (h *AppHandler) getProcessesForAppHandler(authInfo authorization.Info, w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	w.Header().Set("Content-Type", "application/json")

	vars := mux.Vars(r)
	appGUID := vars["guid"]

	app, err := h.appRepo.GetApp(ctx, authInfo, appGUID)
	if err != nil {
		h.handleGetAppErr(err, w, appGUID)
		return
	}

	fetchProcessesForAppMessage := repositories.ListProcessesMessage{
		AppGUID:   []string{appGUID},
		SpaceGUID: app.SpaceGUID,
	}

	processList, err := h.processRepo.ListProcesses(ctx, authInfo, fetchProcessesForAppMessage)
	if err != nil {
		h.logger.Error(err, "Failed to fetch app Process(es) from Kubernetes")
		writeUnknownErrorResponse(w)
		return
	}

	writeResponse(w, http.StatusOK, presenter.ForProcessList(processList, h.serverURL, *r.URL))
}

func (h *AppHandler) getRoutesForAppHandler(authInfo authorization.Info, w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	w.Header().Set("Content-Type", "application/json")

	vars := mux.Vars(r)
	appGUID := vars["guid"]

	app, err := h.appRepo.GetApp(ctx, authInfo, appGUID)
	if err != nil {
		h.handleGetAppErr(err, w, appGUID)
		return
	}

	routes, err := h.lookupAppRouteAndDomainList(ctx, authInfo, app.GUID, app.SpaceGUID)
	if err != nil {
		h.logger.Error(err, "Failed to fetch route or domains from Kubernetes")
		writeUnknownErrorResponse(w)
		return
	}

	writeResponse(w, http.StatusOK, presenter.ForRouteList(routes, h.serverURL, *r.URL))
}

func (h *AppHandler) appScaleProcessHandler(authInfo authorization.Info, w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	w.Header().Set("Content-Type", "application/json")

	vars := mux.Vars(r)
	appGUID := vars["guid"]
	processType := vars["processType"]

	var payload payloads.ProcessScale
	rme := h.decoderValidator.DecodeAndValidateJSONPayload(r, &payload)
	if rme != nil {
		writeRequestMalformedErrorResponse(w, rme)
		return
	}

	processRecord, err := h.scaleAppProcess(ctx, authInfo, appGUID, processType, payload.ToRecord())
	if err != nil {
		switch errType := err.(type) {
		case repositories.NotFoundError:
			resourceType := errType.ResourceType()
			h.logger.Info(fmt.Sprintf("%s not found", resourceType), "appGUID", appGUID)
			writeNotFoundErrorResponse(w, resourceType)
			return
		default:
			h.logger.Error(err, "Failed due to error from Kubernetes", "appGUID", appGUID)
			writeUnknownErrorResponse(w)
			return
		}
	}

	writeResponse(w, http.StatusOK, presenter.ForProcess(processRecord, h.serverURL))
}

func (h *AppHandler) appRestartHandler(authInfo authorization.Info, w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	w.Header().Set("Content-Type", "application/json")

	vars := mux.Vars(r)
	appGUID := vars["guid"]

	app, err := h.appRepo.GetApp(ctx, authInfo, appGUID)
	if err != nil {
		h.handleGetAppErr(err, w, appGUID)
		return
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
			switch err.(type) {
			case repositories.ForbiddenError:
				h.logger.Info("failed to stop app", "AppGUID", appGUID)
				writeNotAuthorizedErrorResponse(w)
				return
			default:
				h.logger.Error(err, "Failed to update app in Kubernetes", "AppGUID", appGUID)
				writeUnknownErrorResponse(w)
				return
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
			writeNotAuthorizedErrorResponse(w)
			return
		default:
			h.logger.Error(err, "Failed to update app in Kubernetes", "AppGUID", appGUID)
			writeUnknownErrorResponse(w)
			return
		}
	}

	writeResponse(w, http.StatusOK, presenter.ForApp(app, h.serverURL))
}

func (h *AppHandler) appDeleteHandler(authInfo authorization.Info, w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vars := mux.Vars(r)
	appGUID := vars["guid"]

	app, err := h.appRepo.GetApp(ctx, authInfo, appGUID)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		h.handleGetAppErr(err, w, appGUID)
		return
	}

	err = h.appRepo.DeleteApp(ctx, authInfo, repositories.DeleteAppMessage{
		AppGUID:   appGUID,
		SpaceGUID: app.SpaceGUID,
	})
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		h.logger.Error(err, "Failed to delete app", "AppGUID", appGUID)
		writeUnknownErrorResponse(w)
	}

	w.Header().Set("Location", fmt.Sprintf("%s/v3/jobs/app.delete-%s", h.serverURL.String(), appGUID))
	w.WriteHeader(http.StatusAccepted)
}

func (h *AppHandler) lookupAppRouteAndDomainList(ctx context.Context, authInfo authorization.Info, appGUID, spaceGUID string) ([]repositories.RouteRecord, error) {
	routeRecords, err := h.routeRepo.ListRoutesForApp(ctx, authInfo, appGUID, spaceGUID)
	if err != nil {
		return []repositories.RouteRecord{}, err
	}

	return getDomainsForRoutes(ctx, h.domainRepo, authInfo, routeRecords)
}

func (h *AppHandler) appPatchEnvVarsHandler(authInfo authorization.Info, w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	ctx := r.Context()
	vars := mux.Vars(r)
	appGUID := vars["guid"]

	var payload payloads.AppPatchEnvVars
	rme := h.decoderValidator.DecodeAndValidateJSONPayload(r, &payload)
	if rme != nil {
		writeRequestMalformedErrorResponse(w, rme)
		return
	}

	app, err := h.appRepo.GetApp(ctx, authInfo, appGUID)
	if err != nil {
		h.handleGetAppErr(err, w, appGUID)
		return
	}

	envVarsRecord, err := h.appRepo.PatchAppEnvVars(ctx, authInfo, payload.ToMessage(appGUID, app.SpaceGUID))
	if err != nil {
		h.logger.Error(err, "Error updating app environment variables")
		writeUnknownErrorResponse(w)
		return
	}

	writeResponse(w, http.StatusOK, presenter.ForAppEnvVars(envVarsRecord, h.serverURL))
}

func (h *AppHandler) appGetEnvHandler(authInfo authorization.Info, w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	vars := mux.Vars(r)
	appGUID := vars["guid"]

	envVars, err := h.appRepo.GetAppEnv(r.Context(), authInfo, appGUID)
	if err != nil {
		switch err.(type) {
		case repositories.NotFoundError:
			h.logger.Error(err, "App not found", "AppGUID", appGUID)
			writeNotFoundErrorResponse(w, "App")
			return
		case repositories.ForbiddenError:
			h.logger.Error(err, "not allowed to fetch App env")
			writeNotAuthorizedErrorResponse(w)
			return
		default:
			h.logger.Error(err, "Failed to fetch app environment variables", "AppGUID", appGUID)
			writeUnknownErrorResponse(w)
			return
		}
	}

	writeResponse(w, http.StatusOK, presenter.ForAppEnv(envVars))
}

func (h *AppHandler) getProcessByTypeForAppHander(authInfo authorization.Info, w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	w.Header().Set("Content-Type", "application/json")

	vars := mux.Vars(r)
	appGUID := vars["guid"]
	processType := vars["type"]

	app, err := h.appRepo.GetApp(ctx, authInfo, appGUID)
	if err != nil {
		h.handleGetAppErr(err, w, appGUID)
		return
	}

	process, err := h.processRepo.GetProcessByAppTypeAndSpace(ctx, authInfo, appGUID, processType, app.SpaceGUID)
	if err != nil {
		switch err.(type) {
		case repositories.NotFoundError:
			h.logger.Info("Process not found", "AppGUID", appGUID)
			writeNotFoundErrorResponse(w, "Process")
		case repositories.ForbiddenError:
			h.logger.Info("Process forbidden", "AppGUID", appGUID)
			writeNotAuthorizedErrorResponse(w)
		default:
			h.logger.Error(err, "Failed to fetch process from Kubernetes", "AppGUID", appGUID)
			writeUnknownErrorResponse(w)
		}
		return
	}

	writeResponse(w, http.StatusOK, presenter.ForProcess(process, h.serverURL))
}

func (h *AppHandler) handleGetAppErr(err error, w http.ResponseWriter, appGUID string) {
	switch err.(type) {
	case repositories.NotFoundError:
		h.logger.Info("App not found", "AppGUID", appGUID)
		writeNotFoundErrorResponse(w, "App")
	case repositories.ForbiddenError:
		h.logger.Info("App forbidden", "AppGUID", appGUID)
		writeNotFoundErrorResponse(w, "App")
	default:
		h.logger.Error(err, "Failed to fetch app from Kubernetes", "AppGUID", appGUID)
		writeUnknownErrorResponse(w)
	}
}

func (h *AppHandler) RegisterRoutes(router *mux.Router) {
	w := NewAuthAwareHandlerFuncWrapper(h.logger)
	router.Path(AppGetEndpoint).Methods("GET").HandlerFunc(w.Wrap(h.appGetHandler))
	router.Path(AppListEndpoint).Methods("GET").HandlerFunc(w.Wrap(h.appListHandler))
	router.Path(AppCreateEndpoint).Methods("POST").HandlerFunc(w.Wrap(h.appCreateHandler))
	router.Path(AppSetCurrentDropletEndpoint).Methods("PATCH").HandlerFunc(w.Wrap(h.appSetCurrentDropletHandler))
	router.Path(AppGetCurrentDropletEndpoint).Methods("GET").HandlerFunc(w.Wrap(h.appGetCurrentDropletHandler))
	router.Path(AppStartEndpoint).Methods("POST").HandlerFunc(w.Wrap(h.appStartHandler))
	router.Path(AppStopEndpoint).Methods("POST").HandlerFunc(w.Wrap(h.appStopHandler))
	router.Path(AppRestartEndpoint).Methods("POST").HandlerFunc(w.Wrap(h.appRestartHandler))
	router.Path(AppProcessScaleEndpoint).Methods("POST").HandlerFunc(w.Wrap(h.appScaleProcessHandler))
	router.Path(AppGetProcessesEndpoint).Methods("GET").HandlerFunc(w.Wrap(h.getProcessesForAppHandler))
	router.Path(AppGetProcessByTypeEndpoint).Methods("GET").HandlerFunc(w.Wrap(h.getProcessByTypeForAppHander))
	router.Path(AppGetRoutesEndpoint).Methods("GET").HandlerFunc(w.Wrap(h.getRoutesForAppHandler))
	router.Path(AppDeleteEndpoint).Methods("DELETE").HandlerFunc(w.Wrap(h.appDeleteHandler))
	router.Path(AppPatchEnvVarsEndpoint).Methods("PATCH").HandlerFunc(w.Wrap(h.appPatchEnvVarsHandler))
	router.Path(AppGetEnvEndpoint).Methods("GET").HandlerFunc(w.Wrap(h.appGetEnvHandler))
}
