package apis

import (
	"context"
	"net/http"
	"net/url"

	"code.cloudfoundry.org/cf-k8s-controllers/api/authorization"
	"code.cloudfoundry.org/cf-k8s-controllers/api/payloads"

	"code.cloudfoundry.org/cf-k8s-controllers/api/presenter"
	"code.cloudfoundry.org/cf-k8s-controllers/api/repositories"

	"github.com/go-logr/logr"
	"github.com/gorilla/mux"
)

const (
	BuildGetEndpoint    = "/v3/builds/{guid}"
	BuildCreateEndpoint = "/v3/builds"
)

//counterfeiter:generate -o fake -fake-name CFBuildRepository . CFBuildRepository
type CFBuildRepository interface {
	FetchBuild(context.Context, authorization.Info, string) (repositories.BuildRecord, error)
	CreateBuild(context.Context, authorization.Info, repositories.BuildCreateMessage) (repositories.BuildRecord, error)
}

type BuildHandler struct {
	serverURL   url.URL
	buildRepo   CFBuildRepository
	packageRepo CFPackageRepository
	logger      logr.Logger
}

func NewBuildHandler(
	logger logr.Logger,
	serverURL url.URL,
	buildRepo CFBuildRepository,
	packageRepo CFPackageRepository,
) *BuildHandler {
	return &BuildHandler{
		logger:      logger,
		serverURL:   serverURL,
		buildRepo:   buildRepo,
		packageRepo: packageRepo,
	}
}

func (h *BuildHandler) buildGetHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	w.Header().Set("Content-Type", "application/json")

	vars := mux.Vars(r)
	buildGUID := vars["guid"]

	authInfo, ok := authorization.InfoFromContext(r.Context())
	if !ok {
		h.logger.Error(nil, "unable to get auth info")
		writeUnknownErrorResponse(w)
		return
	}

	build, err := h.buildRepo.FetchBuild(ctx, authInfo, buildGUID)
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

	err = writeJsonResponse(w, presenter.ForBuild(build, h.serverURL), http.StatusOK)
	if err != nil {
		h.logger.Error(err, "Failed to render response", "BuildGUID", buildGUID)
		writeUnknownErrorResponse(w)
	}
}

func (h *BuildHandler) buildCreateHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	var payload payloads.BuildCreate
	rme := decodeAndValidateJSONPayload(r, &payload)
	if rme != nil {
		writeRequestMalformedErrorResponse(w, rme)
		return
	}

	authInfo, ok := authorization.InfoFromContext(r.Context())
	if !ok {
		h.logger.Error(nil, "unable to get auth info")
		writeUnknownErrorResponse(w)
		return
	}

	packageRecord, err := h.packageRepo.FetchPackage(r.Context(), authInfo, payload.Package.GUID)
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

	buildCreateMessage := payload.ToMessage(packageRecord)

	record, err := h.buildRepo.CreateBuild(r.Context(), authInfo, buildCreateMessage)
	if err != nil {
		h.logger.Info("Error creating build with repository", "error", err.Error())
		writeUnknownErrorResponse(w)
		return
	}

	err = writeJsonResponse(w, presenter.ForBuild(record, h.serverURL), http.StatusCreated)
	if err != nil { // untested
		h.logger.Info("Error encoding JSON response", "error", err.Error())
		writeUnknownErrorResponse(w)
	}
}

func (h *BuildHandler) RegisterRoutes(router *mux.Router) {
	router.Path(BuildGetEndpoint).Methods("GET").HandlerFunc(h.buildGetHandler)
	router.Path(BuildCreateEndpoint).Methods("POST").HandlerFunc(h.buildCreateHandler)
}
