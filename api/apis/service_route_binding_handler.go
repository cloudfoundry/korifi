package apis

import (
	"net/http"
	"net/url"

	"code.cloudfoundry.org/cf-k8s-controllers/api/presenter"

	"github.com/go-logr/logr"
	"github.com/gorilla/mux"
)

const (
	ServiceRouteBindingsListEndpoint = "/v3/service_route_bindings"
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

func (h *ServiceRouteBindingHandler) serviceRouteBindingsListHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	writeResponse(w, http.StatusOK, presenter.ForServiceRouteBindingsList(h.serverURL, *r.URL))
}

func (h *ServiceRouteBindingHandler) RegisterRoutes(router *mux.Router) {
	router.Path(ServiceRouteBindingsListEndpoint).Methods("GET").HandlerFunc(h.serviceRouteBindingsListHandler)
}
