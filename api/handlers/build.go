package handlers

import (
	"context"
	"errors"
	"net/http"
	"net/url"

	"code.cloudfoundry.org/korifi/api/authorization"
	apierrors "code.cloudfoundry.org/korifi/api/errors"
	"code.cloudfoundry.org/korifi/api/payloads"
	"code.cloudfoundry.org/korifi/api/presenter"
	"code.cloudfoundry.org/korifi/api/repositories"
	"code.cloudfoundry.org/korifi/api/routing"

	"github.com/go-logr/logr"
)

const (
	BuildPath  = "/v3/builds/{guid}"
	BuildsPath = "/v3/builds"
)

//counterfeiter:generate -o fake -fake-name CFBuildRepository . CFBuildRepository
type CFBuildRepository interface {
	GetBuild(context.Context, authorization.Info, string) (repositories.BuildRecord, error)
	ListBuilds(context.Context, authorization.Info, repositories.ListBuildsMessage) ([]repositories.BuildRecord, error)
	GetLatestBuildByAppGUID(context.Context, authorization.Info, string, string) (repositories.BuildRecord, error)
	CreateBuild(context.Context, authorization.Info, repositories.CreateBuildMessage) (repositories.BuildRecord, error)
}

type Build struct {
	serverURL        url.URL
	buildRepo        CFBuildRepository
	packageRepo      CFPackageRepository
	appRepo          CFAppRepository
	requestValidator RequestValidator
}

func NewBuild(
	serverURL url.URL,
	buildRepo CFBuildRepository,
	packageRepo CFPackageRepository,
	appRepo CFAppRepository,
	requestValidator RequestValidator,
) *Build {
	return &Build{
		serverURL:        serverURL,
		buildRepo:        buildRepo,
		packageRepo:      packageRepo,
		appRepo:          appRepo,
		requestValidator: requestValidator,
	}
}

func (h *Build) get(r *http.Request) (*routing.Response, error) {
	buildGUID := routing.URLParam(r, "guid")
	authInfo, _ := authorization.InfoFromContext(r.Context())
	logger := logr.FromContextOrDiscard(r.Context()).WithName("handlers.build.get")

	build, err := h.buildRepo.GetBuild(r.Context(), authInfo, buildGUID)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, apierrors.ForbiddenAsNotFound(err), "failed to fetch "+repositories.BuildResourceType, "guid", buildGUID)
	}

	return routing.NewResponse(http.StatusOK).WithBody(presenter.ForBuild(build, h.serverURL)), nil
}

func (h *Build) create(r *http.Request) (*routing.Response, error) {
	authInfo, _ := authorization.InfoFromContext(r.Context())
	logger := logr.FromContextOrDiscard(r.Context()).WithName("handlers.build.create")

	var payload payloads.BuildCreate
	if err := h.requestValidator.DecodeAndValidateJSONPayload(r, &payload); err != nil {
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

	buildCreateMessage := payload.ToMessage(appRecord)

	record, err := h.buildRepo.CreateBuild(r.Context(), authInfo, buildCreateMessage)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "Error creating build with repository")
	}

	return routing.NewResponse(http.StatusCreated).WithBody(presenter.ForBuild(record, h.serverURL)), nil
}

func (h *Build) update(r *http.Request) (*routing.Response, error) { //nolint:dupl
	return nil, apierrors.NewUnprocessableEntityError(errors.New("update build failed"), "Labels and annotations are not supported for builds.")
}

func (h *Build) list(r *http.Request) (*routing.Response, error) {
	authInfo, _ := authorization.InfoFromContext(r.Context())
	logger := logr.FromContextOrDiscard(r.Context()).WithName("handlers.build.list")

	payload := new(payloads.BuildList)
	err := h.requestValidator.DecodeAndValidateURLValues(r, payload)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "Unable to decode request query parameters")
	}

	buildList, err := h.buildRepo.ListBuilds(r.Context(), authInfo, payload.ToMessage())
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "Failed to fetch builds from Kubernetes")
	}

	return routing.NewResponse(http.StatusOK).WithBody(presenter.ForList(presenter.ForBuild, buildList, h.serverURL, *r.URL)), nil
}

func (h *Build) UnauthenticatedRoutes() []routing.Route {
	return nil
}

func (h *Build) AuthenticatedRoutes() []routing.Route {
	return []routing.Route{
		{Method: "GET", Pattern: BuildPath, Handler: h.get},
		{Method: "GET", Pattern: BuildsPath, Handler: h.list},
		{Method: "POST", Pattern: BuildsPath, Handler: h.create},
		{Method: "PATCH", Pattern: BuildPath, Handler: h.update},
	}
}
