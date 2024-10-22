package handlers

import (
	"context"
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
	BuildpacksPath = "/v3/buildpacks"
)

//counterfeiter:generate -o fake -fake-name BuildpackRepository . BuildpackRepository
type BuildpackRepository interface {
	ListBuildpacks(ctx context.Context, authInfo authorization.Info, message repositories.ListBuildpacksMessage) ([]repositories.BuildpackRecord, error)
}

type Buildpack struct {
	serverURL        url.URL
	buildpackRepo    BuildpackRepository
	requestValidator RequestValidator
}

func NewBuildpack(
	serverURL url.URL,
	buildpackRepo BuildpackRepository,
	requestValidator RequestValidator,
) *Buildpack {
	return &Buildpack{
		serverURL:        serverURL,
		buildpackRepo:    buildpackRepo,
		requestValidator: requestValidator,
	}
}

func (h *Buildpack) list(r *http.Request) (*routing.Response, error) {
	authInfo, _ := authorization.InfoFromContext(r.Context())
	logger := logr.FromContextOrDiscard(r.Context()).WithName("handlers.build.list")

	payload := new(payloads.BuildpackList)
	if err := h.requestValidator.DecodeAndValidateURLValues(r, payload); err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "Unable to parse request query parameters")
	}

	buildpacks, err := h.buildpackRepo.ListBuildpacks(r.Context(), authInfo, payload.ToMessage())
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "Failed to fetch buildpacks from Kubernetes")
	}

	return routing.NewResponse(http.StatusOK).WithBody(presenter.ForList(presenter.ForBuildpack, buildpacks, h.serverURL, *r.URL)), nil
}

func (h *Buildpack) UnauthenticatedRoutes() []routing.Route {
	return nil
}

func (h *Buildpack) AuthenticatedRoutes() []routing.Route {
	return []routing.Route{
		{Method: "GET", Pattern: BuildpacksPath, Handler: h.list},
	}
}
