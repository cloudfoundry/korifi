package handlers

import (
	"net/http"
	"net/url"

	"code.cloudfoundry.org/korifi/api/presenter"
	"code.cloudfoundry.org/korifi/api/routing"

	"github.com/go-chi/chi"
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

func (h *ServiceRouteBindingHandler) serviceRouteBindingsListHandler(r *http.Request) (*routing.Response, error) {
	return routing.NewHandlerResponse(http.StatusOK).WithBody(presenter.ForServiceRouteBindingsList(h.serverURL, *r.URL)), nil
}

func (h *ServiceRouteBindingHandler) RegisterRoutes(router *chi.Mux) {
	router.Method("GET", ServiceRouteBindingsPath, routing.Handler(h.serviceRouteBindingsListHandler))
}
