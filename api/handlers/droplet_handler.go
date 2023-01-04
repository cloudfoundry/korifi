package handlers

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	"github.com/go-chi/chi"
	"github.com/go-logr/logr"

	"code.cloudfoundry.org/korifi/api/apierrors"
	"code.cloudfoundry.org/korifi/api/authorization"
	"code.cloudfoundry.org/korifi/api/presenter"
	"code.cloudfoundry.org/korifi/api/repositories"
	"code.cloudfoundry.org/korifi/api/routing"
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
}

func NewDropletHandler(
	serverURL url.URL,
	dropletRepo CFDropletRepository,
) *DropletHandler {
	return &DropletHandler{
		serverURL:   serverURL,
		dropletRepo: dropletRepo,
	}
}

func (h *DropletHandler) dropletGetHandler(r *http.Request) (*routing.Response, error) {
	authInfo, _ := authorization.InfoFromContext(r.Context())
	logger := logr.FromContextOrDiscard(r.Context()).WithName("droplet-handler.droplet-get")

	dropletGUID := chi.URLParam(r, "guid")

	droplet, err := h.dropletRepo.GetDroplet(r.Context(), authInfo, dropletGUID)
	if err != nil {
		return nil, apierrors.LogAndReturn(
			logger,
			apierrors.ForbiddenAsNotFound(err),
			fmt.Sprintf("Failed to fetch %s from Kubernetes", repositories.DropletResourceType),
			"guid", dropletGUID,
		)
	}

	return routing.NewHandlerResponse(http.StatusOK).WithBody(presenter.ForDroplet(droplet, h.serverURL)), nil
}

func (h *DropletHandler) RegisterRoutes(router *chi.Mux) {
	router.Method("GET", DropletPath, routing.Handler(h.dropletGetHandler))
}
