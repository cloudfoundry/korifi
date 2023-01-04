package handlers

import (
	"context"
	"net/http"
	"net/url"

	"code.cloudfoundry.org/korifi/api/apierrors"
	"code.cloudfoundry.org/korifi/api/authorization"
	"code.cloudfoundry.org/korifi/api/payloads"
	"code.cloudfoundry.org/korifi/api/presenter"
	"code.cloudfoundry.org/korifi/api/repositories"
	"code.cloudfoundry.org/korifi/api/routing"

	"github.com/go-chi/chi"
	"github.com/go-logr/logr"
)

const (
	BuildpacksPath = "/v3/buildpacks"
)

//counterfeiter:generate -o fake -fake-name BuildpackRepository . BuildpackRepository
type BuildpackRepository interface {
	ListBuildpacks(ctx context.Context, authInfo authorization.Info) ([]repositories.BuildpackRecord, error)
}

type BuildpackHandler struct {
	serverURL     url.URL
	buildpackRepo BuildpackRepository
}

func NewBuildpackHandler(
	serverURL url.URL,
	buildpackRepo BuildpackRepository,
) *BuildpackHandler {
	return &BuildpackHandler{
		serverURL:     serverURL,
		buildpackRepo: buildpackRepo,
	}
}

func (h *BuildpackHandler) buildpackListHandler(r *http.Request) (*routing.Response, error) {
	authInfo, _ := authorization.InfoFromContext(r.Context())
	logger := logr.FromContextOrDiscard(r.Context()).WithName("buildpack-handler.buildpack-list")

	if err := r.ParseForm(); err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "Unable to parse request query parameters")
	}

	buildpackListFilter := new(payloads.BuildpackList)
	err := payloads.Decode(buildpackListFilter, r.Form)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "Unable to decode request query parameters")
	}

	buildpacks, err := h.buildpackRepo.ListBuildpacks(r.Context(), authInfo)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "Failed to fetch buildpacks from Kubernetes")
	}

	return routing.NewHandlerResponse(http.StatusOK).WithBody(presenter.ForBuildpackList(buildpacks, h.serverURL, *r.URL)), nil
}

func (h *BuildpackHandler) RegisterRoutes(router *chi.Mux) {
	router.Method("GET", BuildpacksPath, routing.Handler(h.buildpackListHandler))
}
