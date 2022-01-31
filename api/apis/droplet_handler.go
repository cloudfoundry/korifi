package apis

import (
	"context"
	"net/http"
	"net/url"

	"github.com/go-logr/logr"
	"github.com/gorilla/mux"

	"code.cloudfoundry.org/cf-k8s-controllers/api/authorization"
	"code.cloudfoundry.org/cf-k8s-controllers/api/presenter"
	"code.cloudfoundry.org/cf-k8s-controllers/api/repositories"
)

const (
	DropletGetEndpoint = "/v3/droplets/{guid}"
)

//counterfeiter:generate -o fake -fake-name CFDropletRepository . CFDropletRepository
type CFDropletRepository interface {
	GetDroplet(context.Context, authorization.Info, string) (repositories.DropletRecord, error)
	ListDroplets(context.Context, authorization.Info, repositories.ListDropletsMessage) ([]repositories.DropletRecord, error)
}

type DropletHandler struct {
	serverURL   url.URL
	dropletRepo CFDropletRepository
	logger      logr.Logger
}

func NewDropletHandler(
	logger logr.Logger,
	serverURL url.URL,
	dropletRepo CFDropletRepository,
) *DropletHandler {
	return &DropletHandler{
		logger:      logger,
		serverURL:   serverURL,
		dropletRepo: dropletRepo,
	}
}

func (h *DropletHandler) dropletGetHandler(authInfo authorization.Info, w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	w.Header().Set("Content-Type", "application/json")

	vars := mux.Vars(r)
	dropletGUID := vars["guid"]

	droplet, err := h.dropletRepo.GetDroplet(ctx, authInfo, dropletGUID)
	if err != nil {
		handleRepoErrors(h.logger, err, "droplet", dropletGUID, w)
		return
	}

	err = writeJsonResponse(w, presenter.ForDroplet(droplet, h.serverURL), http.StatusOK)
	if err != nil {
		h.logger.Error(err, "Failed to render response", "dropletGUID", dropletGUID)
		writeUnknownErrorResponse(w)
	}
}

func (h *DropletHandler) RegisterRoutes(router *mux.Router) {
	w := NewAuthAwareHandlerFuncWrapper(h.logger)
	router.Path(DropletGetEndpoint).Methods("GET").HandlerFunc(w.Wrap(h.dropletGetHandler))
}
