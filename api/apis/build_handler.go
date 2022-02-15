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
	GetBuild(context.Context, authorization.Info, string) (repositories.BuildRecord, error)
	CreateBuild(context.Context, authorization.Info, repositories.CreateBuildMessage) (repositories.BuildRecord, error)
}

type BuildHandler struct {
	serverURL        url.URL
	buildRepo        CFBuildRepository
	packageRepo      CFPackageRepository
	logger           logr.Logger
	decoderValidator *DecoderValidator
}

func NewBuildHandler(
	logger logr.Logger,
	serverURL url.URL,
	buildRepo CFBuildRepository,
	packageRepo CFPackageRepository,
	decoderValidator *DecoderValidator,
) *BuildHandler {
	return &BuildHandler{
		logger:           logger,
		serverURL:        serverURL,
		buildRepo:        buildRepo,
		packageRepo:      packageRepo,
		decoderValidator: decoderValidator,
	}
}

func (h *BuildHandler) buildGetHandler(authInfo authorization.Info, w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	w.Header().Set("Content-Type", "application/json")

	vars := mux.Vars(r)
	buildGUID := vars["guid"]

	build, err := h.buildRepo.GetBuild(ctx, authInfo, buildGUID)
	if err != nil {
		handleRepoErrors(h.logger, err, "build", buildGUID, w)
		return
	}

	writeResponse(w, http.StatusOK, presenter.ForBuild(build, h.serverURL))
}

func (h *BuildHandler) buildCreateHandler(authInfo authorization.Info, w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	var payload payloads.BuildCreate
	rme := h.decoderValidator.DecodeAndValidateJSONPayload(r, &payload)
	if rme != nil {
		writeRequestMalformedErrorResponse(w, rme)
		return
	}

	packageRecord, err := h.packageRepo.GetPackage(r.Context(), authInfo, payload.Package.GUID)
	if err != nil {
		switch err.(type) {
		case repositories.ForbiddenError:
			h.logger.Info("Package forbidden", "Package GUID", payload.Package.GUID)
			writeUnprocessableEntityError(w, "Unable to use package. Ensure that the package exists and you have access to it.")
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
		switch err.(type) {
		case repositories.ForbiddenError:
			h.logger.Info("Create build is forbidden to user")
			writeNotAuthorizedErrorResponse(w)
		default:
			h.logger.Info("Error creating build with repository", "error", err.Error())
			writeUnknownErrorResponse(w)
		}
		return
	}

	writeResponse(w, http.StatusCreated, presenter.ForBuild(record, h.serverURL))
}

func (h *BuildHandler) RegisterRoutes(router *mux.Router) {
	w := NewAuthAwareHandlerFuncWrapper(h.logger)
	router.Path(BuildGetEndpoint).Methods("GET").HandlerFunc(w.Wrap(h.buildGetHandler))
	router.Path(BuildCreateEndpoint).Methods("POST").HandlerFunc(w.Wrap(h.buildCreateHandler))
}
