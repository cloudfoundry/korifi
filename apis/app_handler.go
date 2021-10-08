package apis

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"code.cloudfoundry.org/cf-k8s-controllers/webhooks/workloads"

	"code.cloudfoundry.org/cf-k8s-api/payloads"
	"code.cloudfoundry.org/cf-k8s-api/presenter"
	"code.cloudfoundry.org/cf-k8s-api/repositories"

	"github.com/go-logr/logr"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	AppCreateEndpoint            = "/v3/apps"
	AppGetEndpoint               = "/v3/apps/{guid}"
	AppListEndpoint              = "/v3/apps"
	AppSetCurrentDropletEndpoint = "/v3/apps/{guid}/relationships/current_droplet"

	invalidDropletMsg = "Unable to assign current droplet. Ensure the droplet exists and belongs to this app."
)

//counterfeiter:generate -o fake -fake-name CFAppRepository . CFAppRepository
type CFAppRepository interface {
	FetchApp(context.Context, client.Client, string) (repositories.AppRecord, error)
	FetchAppList(context.Context, client.Client) ([]repositories.AppRecord, error)
	FetchNamespace(context.Context, client.Client, string) (repositories.SpaceRecord, error)
	CreateAppEnvironmentVariables(context.Context, client.Client, repositories.AppEnvVarsRecord) (repositories.AppEnvVarsRecord, error)
	CreateApp(context.Context, client.Client, repositories.AppRecord) (repositories.AppRecord, error)
	SetCurrentDroplet(context.Context, client.Client, repositories.SetCurrentDropletMessage) (repositories.CurrentDropletRecord, error)
}

type AppHandler struct {
	logger      logr.Logger
	serverURL   string
	appRepo     CFAppRepository
	dropletRepo CFDropletRepository
	buildClient ClientBuilder
	k8sConfig   *rest.Config // TODO: this would be global for all requests, not what we want
}

func NewAppHandler(logger logr.Logger, serverURL string, appRepo CFAppRepository, dropletRepo CFDropletRepository, buildClient ClientBuilder, k8sConfig *rest.Config) *AppHandler {
	return &AppHandler{
		logger:      logger,
		serverURL:   serverURL,
		appRepo:     appRepo,
		dropletRepo: dropletRepo,
		buildClient: buildClient,
		k8sConfig:   k8sConfig,
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
	rme := DecodeAndValidatePayload(r, &payload)
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

	appGUID := uuid.New().String()
	var appEnvSecretName string

	if len(payload.EnvironmentVariables) > 0 {
		appEnvSecretRecord := repositories.AppEnvVarsRecord{
			AppGUID:              appGUID,
			SpaceGUID:            namespaceGUID,
			EnvironmentVariables: payload.EnvironmentVariables,
		}
		responseAppEnvSecretRecord, err := h.appRepo.CreateAppEnvironmentVariables(ctx, client, appEnvSecretRecord)
		if err != nil {
			h.logger.Error(err, "Failed to create app environment vars", "App Name", payload.Name)
			writeUnknownErrorResponse(w)
			return
		}
		appEnvSecretName = responseAppEnvSecretRecord.Name
	}

	createAppRecord := payload.ToRecord()
	// Set GUID and EnvSecretName
	createAppRecord.GUID = appGUID
	createAppRecord.EnvSecretName = appEnvSecretName

	responseAppRecord, err := h.appRepo.CreateApp(ctx, client, createAppRecord)
	if err != nil {
		if statusError := new(k8serrors.StatusError); errors.As(err, &statusError) {
			reason := statusError.Status().Reason

			val := new(workloads.ValidationErrorCode)
			val.Unmarshall(string(reason))

			if *val == workloads.DuplicateAppError {
				errorDetail := fmt.Sprintf("App with the name '%s' already exists.", payload.Name)
				h.logger.Error(err, errorDetail, "App Name", payload.Name)
				writeUniquenessError(w, errorDetail)
				return
			}
		}
		h.logger.Error(err, "Failed to create app", "App Name", payload.Name)
		writeUnknownErrorResponse(w)
		return
	}

	responseBody, err := json.Marshal(presenter.ForApp(responseAppRecord, h.serverURL))
	if err != nil {
		h.logger.Error(err, "Failed to render response", "App Name", payload.Name)
		writeUnknownErrorResponse(w)
		return
	}

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
	rme := DecodeAndValidatePayload(r, &payload)
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
			writeNotFoundErrorResponse(w, "App")
		} else {
			h.logger.Error(err, "Error fetching app")
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

func (h *AppHandler) RegisterRoutes(router *mux.Router) {
	router.Path(AppGetEndpoint).Methods("GET").HandlerFunc(h.appGetHandler)
	router.Path(AppListEndpoint).Methods("GET").HandlerFunc(h.appListHandler)
	router.Path(AppCreateEndpoint).Methods("POST").HandlerFunc(h.appCreateHandler)
	router.Path(AppSetCurrentDropletEndpoint).Methods("PATCH").HandlerFunc(h.appSetCurrentDropletHandler)
}
