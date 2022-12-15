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

	"github.com/go-chi/chi"
	"github.com/go-logr/logr"
	ctrl "sigs.k8s.io/controller-runtime"
)

const (
	BuildpacksPath = "/v3/buildpacks"
)

//counterfeiter:generate -o fake -fake-name BuildpackRepository . BuildpackRepository
type BuildpackRepository interface {
	ListBuildpacks(ctx context.Context, authInfo authorization.Info) ([]repositories.BuildpackRecord, error)
}

type BuildpackHandler struct {
	handlerWrapper *AuthAwareHandlerFuncWrapper
	serverURL      url.URL
	buildpackRepo  BuildpackRepository
}

func NewBuildpackHandler(
	serverURL url.URL,
	buildpackRepo BuildpackRepository,
) *BuildpackHandler {
	return &BuildpackHandler{
		handlerWrapper: NewAuthAwareHandlerFuncWrapper(ctrl.Log.WithName("BuildpackHandler")),
		serverURL:      serverURL,
		buildpackRepo:  buildpackRepo,
	}
}

func (h *BuildpackHandler) buildpackListHandler(ctx context.Context, logger logr.Logger, authInfo authorization.Info, r *http.Request) (*HandlerResponse, error) {
	if err := r.ParseForm(); err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "Unable to parse request query parameters")
	}

	buildpackListFilter := new(payloads.BuildpackList)
	err := payloads.Decode(buildpackListFilter, r.Form)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "Unable to decode request query parameters")
	}

	buildpacks, err := h.buildpackRepo.ListBuildpacks(ctx, authInfo)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "Failed to fetch buildpacks from Kubernetes")
	}

	return NewHandlerResponse(http.StatusOK).WithBody(presenter.ForBuildpackList(buildpacks, h.serverURL, *r.URL)), nil
}

func (h *BuildpackHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	router := chi.NewRouter()
	router.Get(BuildpacksPath, h.handlerWrapper.Wrap(h.buildpackListHandler))
	router.ServeHTTP(w, r)
}
