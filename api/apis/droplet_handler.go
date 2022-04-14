package apis

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	"github.com/go-logr/logr"
	"github.com/gorilla/mux"

	"code.cloudfoundry.org/korifi/api/apierrors"
	"code.cloudfoundry.org/korifi/api/authorization"
	"code.cloudfoundry.org/korifi/api/presenter"
	"code.cloudfoundry.org/korifi/api/repositories"
)

const (
	DropletPath = "/v3/droplets/{guid}"
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

func (h *DropletHandler) dropletGetHandler(authInfo authorization.Info, r *http.Request) (*HandlerResponse, error) {
	ctx := r.Context()

	vars := mux.Vars(r)
	dropletGUID := vars["guid"]

	droplet, err := h.dropletRepo.GetDroplet(ctx, authInfo, dropletGUID)
	if err != nil {
		h.logger.Error(err, fmt.Sprintf("Failed to fetch %s from Kubernetes", repositories.DropletResourceType), "guid", dropletGUID)
		return nil, apierrors.ForbiddenAsNotFound(err)
	}

	return NewHandlerResponse(http.StatusOK).WithBody(presenter.ForDroplet(droplet, h.serverURL)), nil
}

func (h *DropletHandler) RegisterRoutes(router *mux.Router) {
	w := NewAuthAwareHandlerFuncWrapper(h.logger)
	router.Path(DropletPath).Methods("GET").HandlerFunc(w.Wrap(h.dropletGetHandler))
}
