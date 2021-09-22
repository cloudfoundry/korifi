package apis

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"code.cloudfoundry.org/cf-k8s-api/message"
	"code.cloudfoundry.org/cf-k8s-api/presenter"
	"code.cloudfoundry.org/cf-k8s-api/repositories"

	"github.com/go-logr/logr"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

//counterfeiter:generate -o fake -fake-name CFAppRepository . CFAppRepository
type CFAppRepository interface {
	FetchApp(context.Context, client.Client, string) (repositories.AppRecord, error)
	FetchAppList(context.Context, client.Client) ([]repositories.AppRecord, error)
	FetchNamespace(context.Context, client.Client, string) (repositories.SpaceRecord, error)
	CreateAppEnvironmentVariables(context.Context, client.Client, repositories.AppEnvVarsRecord) (repositories.AppEnvVarsRecord, error)
	CreateApp(context.Context, client.Client, repositories.AppRecord) (repositories.AppRecord, error)
}

type AppHandler struct {
	ServerURL   string
	AppRepo     CFAppRepository
	BuildClient ClientBuilder
	Logger      logr.Logger
	K8sConfig   *rest.Config // TODO: this would be global for all requests, not what we want
}

func (h *AppHandler) AppGetHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	w.Header().Set("Content-Type", "application/json")

	vars := mux.Vars(r)
	appGUID := vars["guid"]

	// TODO: Instantiate config based on bearer token
	// Spike code from EMEA folks around this: https://github.com/cloudfoundry/cf-crd-explorations/blob/136417fbff507eb13c92cd67e6fed6b061071941/cfshim/handlers/app_handler.go#L78
	client, err := h.BuildClient(h.K8sConfig)
	if err != nil {
		h.Logger.Error(err, "Unable to create Kubernetes client", "AppGUID", appGUID)
		writeUnknownErrorResponse(w)
		return
	}

	app, err := h.AppRepo.FetchApp(ctx, client, appGUID)
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

	responseBody, err := json.Marshal(presenter.ForApp(app, h.ServerURL))
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

	var appCreateMessage message.AppCreateMessage
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
	client, err := h.BuildClient(h.K8sConfig)
	if err != nil {
		h.Logger.Error(err, "Unable to create Kubernetes client")
		writeUnknownErrorResponse(w)
		return
	}

	namespaceGUID := appCreateMessage.Relationships.Space.Data.GUID
	_, err = h.AppRepo.FetchNamespace(ctx, client, namespaceGUID)
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

	appGUID := uuid.New().String()
	var appEnvSecretName string

	if len(appCreateMessage.EnvironmentVariables) > 0 {
		appEnvSecretRecord := repositories.AppEnvVarsRecord{
			AppGUID:              appGUID,
			SpaceGUID:            namespaceGUID,
			EnvironmentVariables: appCreateMessage.EnvironmentVariables,
		}
		responseAppEnvSecretRecord, err := h.AppRepo.CreateAppEnvironmentVariables(ctx, client, appEnvSecretRecord)
		if err != nil {
			h.Logger.Error(err, "Failed to create app environment vars", "App Name", appCreateMessage.Name)
			writeUnknownErrorResponse(w)
			return
		}
		appEnvSecretName = responseAppEnvSecretRecord.Name
	}

	createAppRecord := message.AppCreateMessageToAppRecord(appCreateMessage)
	// Set GUID and EnvSecretName
	createAppRecord.GUID = appGUID
	createAppRecord.EnvSecretName = appEnvSecretName

	responseAppRecord, err := h.AppRepo.CreateApp(ctx, client, createAppRecord)
	if err != nil {
		if errType, ok := err.(*k8serrors.StatusError); ok {
			reason := errType.Status().Reason
			if reason == "CFApp with the same spec.name exists" {
				errorDetail := fmt.Sprintf("App with the name '%s' already exists.", appCreateMessage.Name)
				h.Logger.Error(err, errorDetail, "App Name", appCreateMessage.Name)
				writeUniquenessError(w, errorDetail)
				return
			}
		}
		h.Logger.Error(err, "Failed to create app", "App Name", appCreateMessage.Name)
		writeUnknownErrorResponse(w)
		return
	}

	responseBody, err := json.Marshal(presenter.ForApp(responseAppRecord, h.ServerURL))
	if err != nil {
		h.Logger.Error(err, "Failed to render response", "App Name", appCreateMessage.Name)
		writeUnknownErrorResponse(w)
		return
	}

	w.Write(responseBody)
}

func (h *AppHandler) AppListHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	w.Header().Set("Content-Type", "application/json")

	// TODO: Instantiate config based on bearer token
	// Spike code from EMEA folks around this: https://github.com/cloudfoundry/cf-crd-explorations/blob/136417fbff507eb13c92cd67e6fed6b061071941/cfshim/handlers/app_handler.go#L78
	client, err := h.BuildClient(h.K8sConfig)
	if err != nil {
		h.Logger.Error(err, "Unable to create Kubernetes client")
		writeUnknownErrorResponse(w)
		return
	}

	appList, err := h.AppRepo.FetchAppList(ctx, client)
	if err != nil {
		h.Logger.Error(err, "Failed to fetch app(s) from Kubernetes")
		writeUnknownErrorResponse(w)
		return
	}

	responseBody, err := json.Marshal(presenter.ForAppList(appList, h.ServerURL))
	if err != nil {
		h.Logger.Error(err, "Failed to render response")
		writeUnknownErrorResponse(w)
		return
	}

	w.Write(responseBody)
}
