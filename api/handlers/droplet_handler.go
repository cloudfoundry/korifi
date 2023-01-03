package handlers

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	"github.com/go-logr/logr"

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

func (h *DropletHandler) dropletGetHandler(ctx context.Context, logger logr.Logger, authInfo authorization.Info, r *http.Request) (*HandlerResponse, error) {
	dropletGUID := URLParam(r, "guid")

	droplet, err := h.dropletRepo.GetDroplet(ctx, authInfo, dropletGUID)
	if err != nil {
		return nil, apierrors.LogAndReturn(
			logger,
			apierrors.ForbiddenAsNotFound(err),
			fmt.Sprintf("Failed to fetch %s from Kubernetes", repositories.DropletResourceType),
			"guid", dropletGUID,
		)
	}

	return NewHandlerResponse(http.StatusOK).WithBody(presenter.ForDroplet(droplet, h.serverURL)), nil
}

func (h *DropletHandler) UnauthenticatedRoutes() []Route {
	return []Route{}
}

func (h *DropletHandler) AuthenticatedRoutes() []AuthRoute {
	return []AuthRoute{
		{Method: "GET", Pattern: DropletPath, HandlerFunc: h.dropletGetHandler},
	}
}
