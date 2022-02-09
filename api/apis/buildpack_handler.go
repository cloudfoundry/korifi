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
	"github.com/gorilla/schema"
)

const (
	BuildpackListEndpoint = "/v3/buildpacks"
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

func (h *BuildpackHandler) buildpackListHandler(authInfo authorization.Info, w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	w.Header().Set("Content-Type", "application/json")

	if err := r.ParseForm(); err != nil {
		h.logger.Error(err, "Unable to parse request query parameters")
		writeUnknownErrorResponse(w)
		return
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
					writeUnknownKeyError(w, buildpackListFilter.SupportedQueryParams())
					return
				}
			}

			h.logger.Error(err, "Unable to decode request query parameters")
			writeUnknownErrorResponse(w)
		default:
			h.logger.Error(err, "Unable to decode request query parameters")
			writeUnknownErrorResponse(w)
		}
		return
	}

	buildpacks, err := h.buildpackRepo.GetBuildpacksForBuilder(ctx, authInfo, h.clusterBuilderName)
	if err != nil {
		h.logger.Error(err, "Failed to fetch buildpacks from Kubernetes")
		writeUnknownErrorResponse(w)
		return
	}

	writeResponse(w, http.StatusOK, presenter.ForBuildpackList(buildpacks, h.serverURL, *r.URL))
}

func (h *BuildpackHandler) RegisterRoutes(router *mux.Router) {
	w := NewAuthAwareHandlerFuncWrapper(h.logger)
	router.Path(BuildpackListEndpoint).Methods("GET").HandlerFunc(w.Wrap(h.buildpackListHandler))
}
