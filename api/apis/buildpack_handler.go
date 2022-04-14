package apis

import (
	"context"
	"net/http"
	"net/url"

	"code.cloudfoundry.org/korifi/api/apierrors"
	"code.cloudfoundry.org/korifi/api/authorization"
	"code.cloudfoundry.org/korifi/api/payloads"
	"code.cloudfoundry.org/korifi/api/presenter"
	"code.cloudfoundry.org/korifi/api/repositories"
	"github.com/go-logr/logr"
	"github.com/gorilla/mux"
	"github.com/gorilla/schema"
)

const (
	BuildpacksPath = "/v3/buildpacks"
)

//counterfeiter:generate -o fake -fake-name BuildpackRepository . BuildpackRepository
type BuildpackRepository interface {
	GetBuildpacksForBuilder(ctx context.Context, authInfo authorization.Info, builderName string) ([]repositories.BuildpackRecord, error)
}

type BuildpackHandler struct {
	logger             logr.Logger
	serverURL          url.URL
	buildpackRepo      BuildpackRepository
	clusterBuilderName string
}

func NewBuildpackHandler(
	logger logr.Logger,
	serverURL url.URL,
	buildpackRepo BuildpackRepository,
	clusterBuilderName string,
) *BuildpackHandler {
	return &BuildpackHandler{
		logger:             logger,
		serverURL:          serverURL,
		buildpackRepo:      buildpackRepo,
		clusterBuilderName: clusterBuilderName,
	}
}

func (h *BuildpackHandler) buildpackListHandler(authInfo authorization.Info, r *http.Request) (*HandlerResponse, error) {
	ctx := r.Context()

	if err := r.ParseForm(); err != nil {
		h.logger.Error(err, "Unable to parse request query parameters")
		return nil, err
	}

	// TODO: interface for supported keys list so we can turn this block into a helper function to reduce code duplication
	buildpackListFilter := new(payloads.BuildpackList)
	err := schema.NewDecoder().Decode(buildpackListFilter, r.Form)
	if err != nil {
		switch err.(type) {
		case schema.MultiError:
			multiError := err.(schema.MultiError)
			for _, v := range multiError {
				_, ok := v.(schema.UnknownKeyError)
				if ok {
					h.logger.Info("Unknown key used in Buildpacks")
					return nil, apierrors.NewUnknownKeyError(err, buildpackListFilter.SupportedQueryParams())
				}
			}

			h.logger.Error(err, "Unable to decode request query parameters")
			return nil, err
		default:
			h.logger.Error(err, "Unable to decode request query parameters")
			return nil, err
		}
	}

	buildpacks, err := h.buildpackRepo.GetBuildpacksForBuilder(ctx, authInfo, h.clusterBuilderName)
	if err != nil {
		h.logger.Error(err, "Failed to fetch buildpacks from Kubernetes")
		return nil, err
	}

	return NewHandlerResponse(http.StatusOK).WithBody(presenter.ForBuildpackList(buildpacks, h.serverURL, *r.URL)), nil
}

func (h *BuildpackHandler) RegisterRoutes(router *mux.Router) {
	w := NewAuthAwareHandlerFuncWrapper(h.logger)
	router.Path(BuildpacksPath).Methods("GET").HandlerFunc(w.Wrap(h.buildpackListHandler))
}
