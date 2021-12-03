package apis

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"

	"code.cloudfoundry.org/cf-k8s-controllers/api/authorization"
	"code.cloudfoundry.org/cf-k8s-controllers/api/presenter"
	"code.cloudfoundry.org/cf-k8s-controllers/api/repositories"
	"github.com/go-logr/logr"
	"github.com/gorilla/mux"
)

const (
	DropletGetEndpoint = "/v3/droplets/{guid}"
)

//counterfeiter:generate -o fake -fake-name CFDropletRepository . CFDropletRepository
type CFDropletRepository interface {
	FetchDroplet(context.Context, authorization.Info, string) (repositories.DropletRecord, error)
	FetchDropletList(context.Context, authorization.Info, repositories.DropletListMessage) ([]repositories.DropletRecord, error)
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

func (h *DropletHandler) dropletGetHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	w.Header().Set("Content-Type", "application/json")

	vars := mux.Vars(r)
	dropletGUID := vars["guid"]

	authInfo, ok := authorization.InfoFromContext(r.Context())
	if !ok {
		h.logger.Error(nil, "unable to get auth info")
		writeUnknownErrorResponse(w)
		return
	}

	droplet, err := h.dropletRepo.FetchDroplet(ctx, authInfo, dropletGUID)
	if err != nil {
		switch err.(type) {
		case repositories.NotFoundError:
			h.logger.Info("Droplet not found", "dropletGUID", dropletGUID)
			writeNotFoundErrorResponse(w, "Droplet")
			return
		default:
			h.logger.Error(err, "Failed to fetch droplet from Kubernetes", "dropletGUID", dropletGUID)
			writeUnknownErrorResponse(w)
			return
		}
	}

	responseBody, err := json.Marshal(presenter.ForDroplet(droplet, h.serverURL))
	if err != nil {
		h.logger.Error(err, "Failed to render response", "dropletGUID", dropletGUID)
		writeUnknownErrorResponse(w)
		return
	}
	_, _ = w.Write(responseBody)
}

func (h *DropletHandler) RegisterRoutes(router *mux.Router) {
	router.Path(DropletGetEndpoint).Methods("GET").HandlerFunc(h.dropletGetHandler)
}
