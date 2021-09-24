package apis

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/gorilla/mux"

	"github.com/go-logr/logr"

	"code.cloudfoundry.org/cf-k8s-api/message"

	"code.cloudfoundry.org/cf-k8s-api/presenter"

	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"code.cloudfoundry.org/cf-k8s-api/repositories"
)

const (
	PackageCreateEndpoint = "/v3/packages"
)

//counterfeiter:generate -o fake -fake-name CFPackageRepository . CFPackageRepository

type CFPackageRepository interface {
	CreatePackage(context.Context, client.Client, repositories.PackageCreate) (repositories.PackageRecord, error)
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

	var m message.CreatePackageMessage
	rme := DecodePayload(req, &m)
	if rme != nil {
		writeErrorResponse(w, rme)
		return
	}

	client, err := h.BuildClient(h.K8sConfig)
	if err != nil {
		h.Logger.Info("Error building k8s client", err.Error())
		writeUnknownErrorResponse(w)
		return
	}

	appRecord, err := h.AppRepo.FetchApp(req.Context(), client, m.Relationships.App.Data.GUID)
	if err != nil {
		switch err.(type) {
		case repositories.NotFoundError:
			h.Logger.Info("App not found", "App GUID", m.Relationships.App.Data.GUID)
			writeUnprocessableEntityError(w, "App is invalid. Ensure it exists and you have access to it.")
		default:
			h.Logger.Info("Error finding App", "App GUID", m.Relationships.App.Data.GUID)
			writeUnknownErrorResponse(w)
		}
		return
	}

	record, err := h.PackageRepo.CreatePackage(req.Context(), client, m.ToRecord(appRecord.SpaceGUID)) // TODO: think of a better name than "Record"
	if err != nil {
		h.Logger.Info("Error creating package with repository", err.Error())
		writeUnknownErrorResponse(w)
		return
	}

	res := presenter.ForPackage(record, h.ServerURL)
	w.WriteHeader(http.StatusCreated)
	err = json.NewEncoder(w).Encode(res)
	if err != nil { // untested
		h.Logger.Info("Error encoding JSON response", err.Error())
		writeUnknownErrorResponse(w)
		return
	}
}

func (h *PackageHandler) RegisterRoutes(router *mux.Router) {
	router.Path(PackageCreateEndpoint).Methods("POST").HandlerFunc(h.PackageCreateHandler)
}
