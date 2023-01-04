package handlers

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	"github.com/go-chi/chi"
	"github.com/go-logr/logr"

	"code.cloudfoundry.org/korifi/api/authorization"
	apierrors "code.cloudfoundry.org/korifi/api/errors"
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

type Droplet struct {
	serverURL   url.URL
	dropletRepo CFDropletRepository
}

func NewDroplet(
	serverURL url.URL,
	dropletRepo CFDropletRepository,
) *Droplet {
	return &Droplet{
		serverURL:   serverURL,
		dropletRepo: dropletRepo,
	}
}

func (h *Droplet) get(r *http.Request) (*routing.Response, error) {
	authInfo, _ := authorization.InfoFromContext(r.Context())
	logger := logr.FromContextOrDiscard(r.Context()).WithName("handlers.droplet.get")

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

	return routing.NewResponse(http.StatusOK).WithBody(presenter.ForDroplet(droplet, h.serverURL)), nil
}

func (h *Droplet) RegisterRoutes(router *chi.Mux) {
	router.Method("GET", DropletPath, routing.Handler(h.get))
}
