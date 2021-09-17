package apis

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-logr/logr"

	"code.cloudfoundry.org/cf-k8s-api/message"

	"code.cloudfoundry.org/cf-k8s-api/presenter"

	"code.cloudfoundry.org/cf-k8s-api/repositories"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
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

func (p PackageHandler) PackageCreateHandler(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	var m message.CreatePackageMessage
	err := DecodePayload(req, &m)
	if err != nil {
		var rme *requestMalformedError
		if errors.As(err, &rme) {
			writeErrorResponse(w, rme)
		} else {
			p.Logger.Error(err, "Unknown internal server error")
			writeUnknownErrorResponse(w)
		}
		return
	}

	client, err := p.BuildClient(p.K8sConfig)
	if err != nil {
		p.Logger.Info("Error building k8s client", err.Error())
		writeUnknownErrorResponse(w)
		return
	}

	// check for app existence
	appRecord, err := p.AppRepo.FetchApp(req.Context(), client, m.Relationships.App.Data.GUID)
	if err != nil {
		switch err.(type) {
		case repositories.NotFoundError:
			p.Logger.Info("App not found", "App GUID", m.Relationships.App.Data.GUID)
			writeUnprocessableEntityError(w, "App is invalid. Ensure it exists and you have access to it.")
		default:
			p.Logger.Info("Error finding App", "App GUID", m.Relationships.App.Data.GUID)
			writeUnknownErrorResponse(w)
		}
		return
	}

	record, err := p.PackageRepo.CreatePackage(req.Context(), client, m.ToRecord(appRecord.SpaceGUID)) // TODO: think of a better name than "Record"
	if err != nil {
		p.Logger.Info("Error creating package with repository", err.Error())
		writeUnknownErrorResponse(w)
		return
	}

	// convert the Record into a user-facing form (Presenter)
	res := presenter.ForPackage(record, p.ServerURL)
	w.WriteHeader(http.StatusCreated)
	// Send the API response as JSON
	err = json.NewEncoder(w).Encode(res)
	if err != nil { // untested
		p.Logger.Info("Error encoding JSON response", err.Error())
		writeUnknownErrorResponse(w)
		return
	}
}
