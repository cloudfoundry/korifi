package apis

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"

	"code.cloudfoundry.org/cf-k8s-api/payloads"

	"code.cloudfoundry.org/cf-k8s-api/presenter"
	"code.cloudfoundry.org/cf-k8s-api/repositories"

	"github.com/go-logr/logr"
	"github.com/gorilla/mux"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	BuildGetEndpoint    = "/v3/builds/{guid}"
	BuildCreateEndpoint = "/v3/builds"
)

//counterfeiter:generate -o fake -fake-name CFBuildRepository . CFBuildRepository
type CFBuildRepository interface {
	FetchBuild(context.Context, client.Client, string) (repositories.BuildRecord, error)
	CreateBuild(context.Context, client.Client, repositories.BuildCreateMessage) (repositories.BuildRecord, error)
}

type BuildHandler struct {
	serverURL   url.URL
	buildRepo   CFBuildRepository
	buildClient ClientBuilder
	packageRepo CFPackageRepository
	logger      logr.Logger
	k8sConfig   *rest.Config
}

func NewBuildHandler(
	logger logr.Logger,
	serverURL url.URL,
	buildRepo CFBuildRepository,
	packageRepo CFPackageRepository,
	buildClient ClientBuilder,
	k8sConfig *rest.Config) *BuildHandler {
	return &BuildHandler{
		logger:      logger,
		serverURL:   serverURL,
		buildRepo:   buildRepo,
		packageRepo: packageRepo,
		buildClient: buildClient,
		k8sConfig:   k8sConfig,
	}
}

func (h *BuildHandler) buildGetHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	w.Header().Set("Content-Type", "application/json")

	vars := mux.Vars(r)
	buildGUID := vars["guid"]

	// TODO: Instantiate config based on bearer token
	// Spike code from EMEA folks around this: https://github.com/cloudfoundry/cf-crd-explorations/blob/136417fbff507eb13c92cd67e6fed6b061071941/cfshim/handlers/app_handler.go#L78
	client, err := h.buildClient(h.k8sConfig)
	if err != nil {
		h.logger.Error(err, "Unable to create Kubernetes client", "BuildGUID", buildGUID)
		writeUnknownErrorResponse(w)
		return
	}

	build, err := h.buildRepo.FetchBuild(ctx, client, buildGUID)
	if err != nil {
		switch err.(type) {
		case repositories.NotFoundError:
			h.logger.Info("Build not found", "BuildGUID", buildGUID)
			writeNotFoundErrorResponse(w, "Build")
			return
		default:
			h.logger.Error(err, "Failed to fetch build from Kubernetes", "BuildGUID", buildGUID)
			writeUnknownErrorResponse(w)
			return
		}
	}

	responseBody, err := json.Marshal(presenter.ForBuild(build, h.serverURL))
	if err != nil {
		h.logger.Error(err, "Failed to render response", "BuildGUID", buildGUID)
		writeUnknownErrorResponse(w)
		return
	}
	w.Write(responseBody)
}

func (h *BuildHandler) buildCreateHandler(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	var payload payloads.BuildCreate
	rme := DecodeAndValidatePayload(req, &payload)
	if rme != nil {
		writeErrorResponse(w, rme)
		return
	}

	client, err := h.buildClient(h.k8sConfig)
	if err != nil {
		h.logger.Info("Error building k8s client", "error", err.Error())
		writeUnknownErrorResponse(w)
		return
	}

	packageRecord, err := h.packageRepo.FetchPackage(req.Context(), client, payload.Package.GUID)
	if err != nil {
		switch err.(type) {
		case repositories.NotFoundError:
			h.logger.Info("Package not found", "Package GUID", payload.Package.GUID)
			writeUnprocessableEntityError(w, "Unable to use package. Ensure that the package exists and you have access to it.")
		default:
			h.logger.Info("Error finding Package", "Package GUID", payload.Package.GUID)
			writeUnknownErrorResponse(w)
		}
		return
	}

	buildCreateMessage := payload.ToMessage(packageRecord.AppGUID, packageRecord.SpaceGUID)

	record, err := h.buildRepo.CreateBuild(req.Context(), client, buildCreateMessage)
	if err != nil {
		h.logger.Info("Error creating build with repository", "error", err.Error())
		writeUnknownErrorResponse(w)
		return
	}

	res := presenter.ForBuild(record, h.serverURL)
	w.WriteHeader(http.StatusCreated)
	err = json.NewEncoder(w).Encode(res)
	if err != nil { // untested
		h.logger.Info("Error encoding JSON response", "error", err.Error())
		writeUnknownErrorResponse(w)
		return
	}
}

func (h *BuildHandler) RegisterRoutes(router *mux.Router) {
	router.Path(BuildGetEndpoint).Methods("GET").HandlerFunc(h.buildGetHandler)
	router.Path(BuildCreateEndpoint).Methods("POST").HandlerFunc(h.buildCreateHandler)
}
