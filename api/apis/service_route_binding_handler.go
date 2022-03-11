package apis

import (
	"net/http"
	"net/url"

	"code.cloudfoundry.org/cf-k8s-controllers/api/authorization"
	"code.cloudfoundry.org/cf-k8s-controllers/api/presenter"

	"github.com/go-logr/logr"
	"github.com/gorilla/mux"
)

const (
	ServiceRouteBindingsPath = "/v3/service_route_bindings"
)

type ServiceRouteBindingHandler struct {
	logger    logr.Logger
	serverURL url.URL
}

func NewServiceRouteBindingHandler(
	logger logr.Logger,
	serverURL url.URL,
) *ServiceRouteBindingHandler {
	return &ServiceRouteBindingHandler{
		logger:    logger,
		serverURL: serverURL,
	}
}

func (h *ServiceRouteBindingHandler) serviceRouteBindingsListHandler(authInfo authorization.Info, r *http.Request) (*HandlerResponse, error) {
	return NewHandlerResponse(http.StatusOK).WithBody(presenter.ForServiceRouteBindingsList(h.serverURL, *r.URL)), nil
}

func (h *ServiceRouteBindingHandler) RegisterRoutes(router *mux.Router) {
	w := NewAuthAwareHandlerFuncWrapper(h.logger)
	router.Path(ServiceRouteBindingsPath).Methods("GET").HandlerFunc(w.Wrap(h.serviceRouteBindingsListHandler))
}
