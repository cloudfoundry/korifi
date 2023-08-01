package handlers

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	"github.com/go-logr/logr"

	"code.cloudfoundry.org/korifi/api/authorization"
	apierrors "code.cloudfoundry.org/korifi/api/errors"
	"code.cloudfoundry.org/korifi/api/payloads"
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
	UpdateDroplet(context.Context, authorization.Info, repositories.UpdateDropletMessage) (repositories.DropletRecord, error)
}

type Droplet struct {
	serverURL        url.URL
	dropletRepo      CFDropletRepository
	requestValidator RequestValidator
}

func NewDroplet(
	serverURL url.URL,
	dropletRepo CFDropletRepository,
	requestValidator RequestValidator,
) *Droplet {
	return &Droplet{
		serverURL:        serverURL,
		dropletRepo:      dropletRepo,
		requestValidator: requestValidator,
	}
}

func (h *Droplet) get(r *http.Request) (*routing.Response, error) {
	authInfo, _ := authorization.InfoFromContext(r.Context())
	logger := logr.FromContextOrDiscard(r.Context()).WithName("handlers.droplet.get")

	dropletGUID := routing.URLParam(r, "guid")

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

func (h *Droplet) update(r *http.Request) (*routing.Response, error) {
	authInfo, _ := authorization.InfoFromContext(r.Context())
	logger := logr.FromContextOrDiscard(r.Context()).WithName("handlers.droplet.update")

	dropletGUID := routing.URLParam(r, "guid")

	var payload payloads.DropletUpdate
	if err := h.requestValidator.DecodeAndValidateJSONPayload(r, &payload); err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "failed to decode payload")
	}

	_, err := h.dropletRepo.GetDroplet(r.Context(), authInfo, dropletGUID)
	if err != nil {
		return nil, apierrors.LogAndReturn(
			logger,
			apierrors.ForbiddenAsNotFound(err),
			fmt.Sprintf("Failed to fetch %s from Kubernetes", repositories.DropletResourceType),
			"guid", dropletGUID,
		)
	}

	droplet, err := h.dropletRepo.UpdateDroplet(r.Context(), authInfo, payload.ToMessage(dropletGUID))
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "Error updating droplet in repository")
	}

	return routing.NewResponse(http.StatusOK).WithBody(presenter.ForDroplet(droplet, h.serverURL)), nil
}

func (h *Droplet) UnauthenticatedRoutes() []routing.Route {
	return nil
}

func (h *Droplet) AuthenticatedRoutes() []routing.Route {
	return []routing.Route{
		{Method: "GET", Pattern: DropletPath, Handler: h.get},
		{Method: "PATCH", Pattern: DropletPath, Handler: h.update},
	}
}
