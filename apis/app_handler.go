package apis

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/google/uuid"

	"code.cloudfoundry.org/cf-k8s-api/messages"
	"code.cloudfoundry.org/cf-k8s-api/presenters"
	"code.cloudfoundry.org/cf-k8s-api/repositories"
	"github.com/go-logr/logr"
	"github.com/gorilla/mux"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

//go:generate go run github.com/maxbrunsfeld/counterfeiter/v6 . CFAppRepository
type CFAppRepository interface {
	ConfigureClient(*rest.Config) (client.Client, error)
	FetchApp(client.Client, context.Context, string) (repositories.AppRecord, error)
	FetchNamespace(client.Client, context.Context, string) (repositories.SpaceRecord, error)
	AppExists(client.Client, context.Context, string, string) (bool, error)
	CreateAppEnvironmentVariables(client.Client, context.Context, repositories.AppEnvVarsRecord) (repositories.AppEnvVarsRecord, error)
	CreateApp(client.Client, context.Context, repositories.AppRecord) (repositories.AppRecord, error)
}

type AppHandler struct {
	ServerURL string
	AppRepo   CFAppRepository
	Logger    logr.Logger
	K8sConfig *rest.Config // TODO: this would be global for all requests, not what we want
}

func (h *AppHandler) AppGetHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	w.Header().Set("Content-Type", "application/json")

	vars := mux.Vars(r)
	appGUID := vars["guid"]

	// TODO: Instantiate config based on bearer token
	// Spike code from EMEA folks around this: https://github.com/cloudfoundry/cf-crd-explorations/blob/136417fbff507eb13c92cd67e6fed6b061071941/cfshim/handlers/app_handler.go#L78
	client, err := h.AppRepo.ConfigureClient(h.K8sConfig)
	if err != nil {
		h.Logger.Error(err, "Unable to create Kubernetes client", "AppGUID", appGUID)
		writeUnknownErrorResponse(w)
		return
	}

	app, err := h.AppRepo.FetchApp(client, ctx, appGUID)
	if err != nil {
		switch err.(type) {
		case repositories.NotFoundError:
			h.Logger.Info("App not found", "AppGUID", appGUID)
			writeNotFoundErrorResponse(w, "App")
			return
		default:
			h.Logger.Error(err, "Failed to fetch app from Kubernetes", "AppGUID", appGUID)
			writeUnknownErrorResponse(w)
			return
		}
	}

	responseBody, err := json.Marshal(presenters.AppRecordToAppResponse(app, h.ServerURL))
	if err != nil {
		h.Logger.Error(err, "Failed to render response", "AppGUID", appGUID)
		writeUnknownErrorResponse(w)
		return
	}

	w.Write(responseBody)
}

func (h *AppHandler) AppCreateHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	w.Header().Set("Content-Type", "application/json")

	var appCreateMessage messages.AppCreateMessage
	err := DecodePayload(r, &appCreateMessage)
	if err != nil {
		var rme *requestMalformedError
		if errors.As(err, &rme) {
			writeErrorResponse(w, rme)
		} else {
			h.Logger.Error(err, "Unknown internal server error")
			writeUnknownErrorResponse(w)
		}
		return
	}

	// TODO: Instantiate config based on bearer token
	// Spike code from EMEA folks around this: https://github.com/cloudfoundry/cf-crd-explorations/blob/136417fbff507eb13c92cd67e6fed6b061071941/cfshim/handlers/app_handler.go#L78
	client, err := h.AppRepo.ConfigureClient(h.K8sConfig)
	if err != nil {
		h.Logger.Error(err, "Unable to create Kubernetes client")
		writeUnknownErrorResponse(w)
		return
	}

	namespaceGUID := appCreateMessage.Relationships.Space.Data.GUID
	_, err = h.AppRepo.FetchNamespace(client, ctx, namespaceGUID)
	if err != nil {
		switch err.(type) {
		case repositories.PermissionDeniedOrNotFoundError:
			h.Logger.Info("Namespace not found", "Namespace GUID", namespaceGUID)
			writeUnprocessableEntityError(w, "Invalid space. Ensure that the space exists and you have access to it.")
			return
		default:
			h.Logger.Error(err, "Failed to fetch namespace from Kubernetes", "Namespace GUID", namespaceGUID)
			writeUnknownErrorResponse(w)
			return
		}
	}

	appName := appCreateMessage.Name
	appExists, err := h.AppRepo.AppExists(client, ctx, appName, namespaceGUID)
	if err != nil {
		h.Logger.Error(err, "Failed to fetch app from Kubernetes", "App Name", appName)
		writeUnknownErrorResponse(w)
		return
	}

	if appExists {
		errorDetail := fmt.Sprintf("App with the name '%s' already exists.", appName)
		h.Logger.Error(err, errorDetail, "App Name", appName)
		writeUniquenessError(w, errorDetail)
		return
	}

	appGUID := uuid.New().String()
	var appEnvSecretName string

	if len(appCreateMessage.EnvironmentVariables) > 0 {
		appEnvSecretRecord := repositories.AppEnvVarsRecord{
			AppGUID:              appGUID,
			SpaceGUID:            namespaceGUID,
			EnvironmentVariables: appCreateMessage.EnvironmentVariables,
		}
		responseAppEnvSecretRecord, err := h.AppRepo.CreateAppEnvironmentVariables(client, ctx, appEnvSecretRecord)
		if err != nil {
			h.Logger.Error(err, "Failed to create app environment vars", "App Name", appName)
			writeUnknownErrorResponse(w)
			return
		}
		appEnvSecretName = responseAppEnvSecretRecord.Name
	}

	createAppRecord := messages.AppCreateMessageToAppRecord(appCreateMessage)
	// Set GUID and EnvSecretName
	createAppRecord.GUID = appGUID
	createAppRecord.EnvSecretName = appEnvSecretName

	responseAppRecord, err := h.AppRepo.CreateApp(client, ctx, createAppRecord)
	if err != nil {
		h.Logger.Error(err, "Failed to create app", "App Name", appName)
		writeUnknownErrorResponse(w)
		return
	}
	responseBody, err := json.Marshal(presenters.AppRecordToAppResponse(responseAppRecord, h.ServerURL))
	if err != nil {
		h.Logger.Error(err, "Failed to render response", "App Name", appName)
		writeUnknownErrorResponse(w)
		return
	}

	w.Write(responseBody)
}
