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

type ServiceRouteBinding struct {
	serverURL url.URL
}

func NewServiceRouteBinding(
	serverURL url.URL,
) *ServiceRouteBinding {
	return &ServiceRouteBinding{
		serverURL: serverURL,
	}
}

func (h *ServiceRouteBinding) list(r *http.Request) (*routing.Response, error) {
	return routing.NewResponse(http.StatusOK).WithBody(presenter.ForServiceRouteBindingsList(h.serverURL, *r.URL)), nil
}

func (h *ServiceRouteBinding) RegisterRoutes(router *chi.Mux) {
	router.Method("GET", ServiceRouteBindingsPath, routing.Handler(h.list))
}
