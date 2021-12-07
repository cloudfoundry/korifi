package apis

import (
	"encoding/json"
	"net/http"
	"net/url"

	"github.com/go-logr/logr"

	"code.cloudfoundry.org/cf-k8s-controllers/api/presenter"

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
	serverURL url.URL) *ServiceRouteBindingHandler {
	return &ServiceRouteBindingHandler{
		logger:    logger,
		serverURL: serverURL,
	}
}

func (h *ServiceRouteBindingHandler) serviceRouteBindingsListHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	responseBody, err := json.Marshal(presenter.ForServiceRouteBindingsList(h.serverURL))
	if err != nil {
		h.logger.Error(err, "Failed to render response")
		writeUnknownErrorResponse(w)
		return
	}

	_, _ = w.Write(responseBody)
}

func (h *ServiceRouteBindingHandler) RegisterRoutes(router *mux.Router) {
	router.Path(ServiceRouteBindingsListEndpoint).Methods("GET").HandlerFunc(h.serviceRouteBindingsListHandler)
}
