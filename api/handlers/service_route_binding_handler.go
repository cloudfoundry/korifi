package handlers

import (
	"context"
	"net/http"
	"net/url"

	"code.cloudfoundry.org/korifi/api/authorization"
	"code.cloudfoundry.org/korifi/api/presenter"

	"github.com/go-logr/logr"
)

const (
	ServiceRouteBindingsPath = "/v3/service_route_bindings"
)

type ServiceRouteBindingHandler struct {
	serverURL url.URL
}

func NewServiceRouteBindingHandler(
	serverURL url.URL,
) *ServiceRouteBindingHandler {
	return &ServiceRouteBindingHandler{
		serverURL: serverURL,
	}
}

func (h *ServiceRouteBindingHandler) serviceRouteBindingsListHandler(ctx context.Context, logger logr.Logger, authInfo authorization.Info, r *http.Request) (*HandlerResponse, error) {
	return NewHandlerResponse(http.StatusOK).WithBody(presenter.ForServiceRouteBindingsList(h.serverURL, *r.URL)), nil
}

func (h *ServiceRouteBindingHandler) AuthenticatedRoutes() []Route {
	return []Route{
		{Method: "GET", Pattern: ServiceRouteBindingsPath, HandlerFunc: h.serviceRouteBindingsListHandler},
	}
}

func (h *ServiceRouteBindingHandler) UnauthenticatedRoutes() []Route {
	return []Route{}
}
