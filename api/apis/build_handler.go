package apis

import (
	"context"
	"net/http"
	"net/url"

	"code.cloudfoundry.org/cf-k8s-controllers/api/apierrors"
	"code.cloudfoundry.org/cf-k8s-controllers/api/authorization"
	"code.cloudfoundry.org/cf-k8s-controllers/api/payloads"

	"code.cloudfoundry.org/cf-k8s-controllers/api/presenter"
	"code.cloudfoundry.org/cf-k8s-controllers/api/repositories"

	"github.com/go-logr/logr"
	"github.com/gorilla/mux"
)

const (
	BuildPath  = "/v3/builds/{guid}"
	BuildsPath = "/v3/builds"
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

func (h *BuildHandler) buildGetHandler(authInfo authorization.Info, r *http.Request) (*HandlerResponse, error) {
	ctx := r.Context()

	vars := mux.Vars(r)
	buildGUID := vars["guid"]

	build, err := h.buildRepo.GetBuild(ctx, authInfo, buildGUID)
	if err != nil {
		return nil, handleRepoErrors(h.logger, err, repositories.BuildResourceType, buildGUID)
	}

	return NewHandlerResponse(http.StatusOK).WithBody(presenter.ForBuild(build, h.serverURL)), nil
}

func (h *BuildHandler) buildCreateHandler(authInfo authorization.Info, r *http.Request) (*HandlerResponse, error) {
	var payload payloads.BuildCreate
	if err := h.decoderValidator.DecodeAndValidateJSONPayload(r, &payload); err != nil {
		return nil, err
	}

	packageRecord, err := h.packageRepo.GetPackage(r.Context(), authInfo, payload.Package.GUID)
	if err != nil {
		switch err.(type) {
		case repositories.ForbiddenError:
			h.logger.Info("Package forbidden", "Package GUID", payload.Package.GUID)
			return nil, apierrors.NewUnprocessableEntityError(err, "Unable to use package. Ensure that the package exists and you have access to it.")
		case repositories.NotFoundError:
			h.logger.Info("Package not found", "Package GUID", payload.Package.GUID)
			return nil, apierrors.NewUnprocessableEntityError(err, "Unable to use package. Ensure that the package exists and you have access to it.")
		default:
			h.logger.Info("Error finding Package", "Package GUID", payload.Package.GUID)
			return nil, err
		}
	}

	buildCreateMessage := payload.ToMessage(packageRecord)

	record, err := h.buildRepo.CreateBuild(r.Context(), authInfo, buildCreateMessage)
	if err != nil {
		switch err.(type) {
		case repositories.ForbiddenError:
			h.logger.Info("Create build is forbidden to user")
			return nil, apierrors.NewForbiddenError(err, repositories.BuildResourceType)
		default:
			h.logger.Info("Error creating build with repository", "error", err.Error())
			return nil, err
		}
	}

	return NewHandlerResponse(http.StatusCreated).WithBody(presenter.ForBuild(record, h.serverURL)), nil
}

func (h *BuildHandler) RegisterRoutes(router *mux.Router) {
	w := NewAuthAwareHandlerFuncWrapper(h.logger)
	router.Path(BuildPath).Methods("GET").HandlerFunc(w.Wrap(h.buildGetHandler))
	router.Path(BuildsPath).Methods("POST").HandlerFunc(w.Wrap(h.buildCreateHandler))
}
