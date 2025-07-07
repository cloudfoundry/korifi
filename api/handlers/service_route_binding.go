package handlers

import (
	"net/http"
	"net/url"

	"code.cloudfoundry.org/korifi/api/presenter"
	"code.cloudfoundry.org/korifi/api/repositories"
	"code.cloudfoundry.org/korifi/api/repositories/include"
	"code.cloudfoundry.org/korifi/api/repositories/k8sklient/descriptors"
	"code.cloudfoundry.org/korifi/api/routing"
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
	return routing.NewResponse(http.StatusOK).WithBody(presenter.ForList(
		func(a any, _ url.URL, includes ...include.Resource) any { return a },
		repositories.ListResult[any]{
			PageInfo: descriptors.SinglePageInfo(0, 0),
			Records:  []any{},
		},
		h.serverURL,
		*r.URL,
	)), nil
}

func (h *ServiceRouteBinding) UnauthenticatedRoutes() []routing.Route {
	return nil
}

func (h *ServiceRouteBinding) AuthenticatedRoutes() []routing.Route {
	return []routing.Route{
		{Method: "GET", Pattern: ServiceRouteBindingsPath, Handler: h.list},
	}
}
