package apis

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/gorilla/mux"

	"github.com/go-logr/logr"

	"code.cloudfoundry.org/cf-k8s-api/payloads"

	"code.cloudfoundry.org/cf-k8s-api/presenter"

	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"code.cloudfoundry.org/cf-k8s-api/repositories"
)

const (
	PackageCreateEndpoint = "/v3/packages"
	PackageUploadEndpoint = "/v3/packages/{guid}/upload"
)

//counterfeiter:generate -o fake -fake-name CFPackageRepository . CFPackageRepository

type CFPackageRepository interface {
	FetchPackage(context.Context, client.Client, string) (repositories.PackageRecord, error)
	CreatePackage(context.Context, client.Client, repositories.PackageCreateMessage) (repositories.PackageRecord, error)
}

type PackageHandler struct {
	ServerURL   string
	PackageRepo CFPackageRepository
	AppRepo     CFAppRepository
	K8sConfig   *rest.Config
	Logger      logr.Logger
	BuildClient ClientBuilder
}

func (h PackageHandler) PackageCreateHandler(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	var payload payloads.PackageCreate
	rme := DecodeAndValidatePayload(req, &payload)
	if rme != nil {
		writeErrorResponse(w, rme)
		return
	}

	client, err := h.BuildClient(h.K8sConfig)
	if err != nil {
		h.Logger.Info("Error building k8s client", "error", err.Error())
		writeUnknownErrorResponse(w)
		return
	}

	appRecord, err := h.AppRepo.FetchApp(req.Context(), client, payload.Relationships.App.Data.GUID)
	if err != nil {
		switch err.(type) {
		case repositories.NotFoundError:
			h.Logger.Info("App not found", "App GUID", payload.Relationships.App.Data.GUID)
			writeUnprocessableEntityError(w, "App is invalid. Ensure it exists and you have access to it.")
		default:
			h.Logger.Info("Error finding App", "App GUID", payload.Relationships.App.Data.GUID)
			writeUnknownErrorResponse(w)
		}
		return
	}

	record, err := h.PackageRepo.CreatePackage(req.Context(), client, payload.ToMessage(appRecord.SpaceGUID))
	if err != nil {
		h.Logger.Info("Error creating package with repository", "error", err.Error())
		writeUnknownErrorResponse(w)
		return
	}

	res := presenter.ForPackage(record, h.ServerURL)
	w.WriteHeader(http.StatusCreated)
	err = json.NewEncoder(w).Encode(res)
	if err != nil { // untested
		h.Logger.Info("Error encoding JSON response", "error", err.Error())
		writeUnknownErrorResponse(w)
		return
	}
}

func (h PackageHandler) PackageUploadHandler(w http.ResponseWriter, req *http.Request) {
	packageGUID := mux.Vars(req)["guid"]

	w.Header().Set("Content-Type", "application/json")

	client, err := h.BuildClient(h.K8sConfig)
	if err != nil {
		h.Logger.Info("Error building k8s client", "error", err.Error())
		writeUnknownErrorResponse(w)
		return
	}

	record, err := h.PackageRepo.FetchPackage(req.Context(), client, packageGUID)
	if err != nil {
		switch {
		case errors.As(err, new(repositories.NotFoundError)):
			writeNotFoundErrorResponse(w, "Package")
		default:
			h.Logger.Info("Error fetching package with repository", "error", err.Error())
			writeUnknownErrorResponse(w)
		}
		return
	}

	res := presenter.ForPackage(record, h.ServerURL)
	err = json.NewEncoder(w).Encode(res)
	if err != nil { // untested
		h.Logger.Info("Error encoding JSON response", "error", err.Error())
		writeUnknownErrorResponse(w)
		return
	}
}

func (h *PackageHandler) RegisterRoutes(router *mux.Router) {
	router.Path(PackageCreateEndpoint).Methods("POST").HandlerFunc(h.PackageCreateHandler)
	router.Path(PackageUploadEndpoint).Methods("POST").HandlerFunc(h.PackageUploadHandler)
}
