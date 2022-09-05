package handlers

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	"code.cloudfoundry.org/korifi/api/apierrors"
	"code.cloudfoundry.org/korifi/api/authorization"
	"code.cloudfoundry.org/korifi/api/payloads"
	"code.cloudfoundry.org/korifi/api/presenter"
	"code.cloudfoundry.org/korifi/api/repositories"

	"github.com/go-logr/logr"
	"github.com/gorilla/mux"
	ctrl "sigs.k8s.io/controller-runtime"
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
	appRepo          CFAppRepository
	handlerWrapper   *AuthAwareHandlerFuncWrapper
	decoderValidator *DecoderValidator
}

func NewBuildHandler(
	serverURL url.URL,
	buildRepo CFBuildRepository,
	packageRepo CFPackageRepository,
	appRepo CFAppRepository,
	decoderValidator *DecoderValidator,
) *BuildHandler {
	return &BuildHandler{
		handlerWrapper:   NewAuthAwareHandlerFuncWrapper(ctrl.Log.WithName("BuildHandler")),
		serverURL:        serverURL,
		buildRepo:        buildRepo,
		packageRepo:      packageRepo,
		appRepo:          appRepo,
		decoderValidator: decoderValidator,
	}
}

func (h *BuildHandler) buildGetHandler(ctx context.Context, logger logr.Logger, authInfo authorization.Info, r *http.Request) (*HandlerResponse, error) {
	vars := mux.Vars(r)
	buildGUID := vars["guid"]

	build, err := h.buildRepo.GetBuild(ctx, authInfo, buildGUID)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, apierrors.ForbiddenAsNotFound(err), fmt.Sprintf("Failed to fetch %s from Kubernetes", repositories.BuildResourceType), "guid", buildGUID)
	}

	return NewHandlerResponse(http.StatusOK).WithBody(presenter.ForBuild(build, h.serverURL)), nil
}

func (h *BuildHandler) buildCreateHandler(ctx context.Context, logger logr.Logger, authInfo authorization.Info, r *http.Request) (*HandlerResponse, error) {
	var payload payloads.BuildCreate
	if err := h.decoderValidator.DecodeAndValidateJSONPayload(r, &payload); err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "failed to decode payload")
	}

	packageRecord, err := h.packageRepo.GetPackage(r.Context(), authInfo, payload.Package.GUID)
	if err != nil {
		return nil, apierrors.LogAndReturn(
			logger,
			apierrors.AsUnprocessableEntity(err,
				"Unable to use package. Ensure that the package exists and you have access to it.",
				apierrors.ForbiddenError{},
				apierrors.NotFoundError{},
			),
			"Error finding Package", "Package GUID", payload.Package.GUID,
		)
	}

	appRecord, err := h.appRepo.GetApp(r.Context(), authInfo, packageRecord.AppGUID)
	if err != nil {
		return nil, apierrors.LogAndReturn(
			logger,
			apierrors.AsUnprocessableEntity(err,
				"Unable to use the app associated with that package. Ensure that the app exists and you have access to it.",
				apierrors.ForbiddenError{},
				apierrors.NotFoundError{},
			),
			"Error finding App", "App GUID", packageRecord.AppGUID,
		)
	}

	buildCreateMessage := payload.ToMessage(appRecord, packageRecord)

	record, err := h.buildRepo.CreateBuild(r.Context(), authInfo, buildCreateMessage)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "Error creating build with repository")
	}

	return NewHandlerResponse(http.StatusCreated).WithBody(presenter.ForBuild(record, h.serverURL)), nil
}

func (h *BuildHandler) RegisterRoutes(router *mux.Router) {
	router.Path(BuildPath).Methods("GET").HandlerFunc(h.handlerWrapper.Wrap(h.buildGetHandler))
	router.Path(BuildsPath).Methods("POST").HandlerFunc(h.handlerWrapper.Wrap(h.buildCreateHandler))
}
