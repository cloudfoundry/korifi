package handlers

import (
	"context"
	"net/http"
	"net/url"

	"code.cloudfoundry.org/korifi/api/authorization"
	"code.cloudfoundry.org/korifi/api/presenter"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/go-logr/logr"
	"github.com/gorilla/mux"
)

const (
	ServiceRouteBindingsPath = "/v3/service_route_bindings"
)

type ServiceRouteBindingHandler struct {
	handlerWrapper *AuthAwareHandlerFuncWrapper
	serverURL      url.URL
}

func NewServiceRouteBindingHandler(
	serverURL url.URL,
) *ServiceRouteBindingHandler {
	return &ServiceRouteBindingHandler{
		handlerWrapper: NewAuthAwareHandlerFuncWrapper(ctrl.Log.WithName("ServiceRouteBindingHandler")),
		serverURL:      serverURL,
	}
}

func (h *ServiceRouteBindingHandler) serviceRouteBindingsListHandler(ctx context.Context, logger logr.Logger, authInfo authorization.Info, r *http.Request) (*HandlerResponse, error) {
	return NewHandlerResponse(http.StatusOK).WithBody(presenter.ForServiceRouteBindingsList(h.serverURL, *r.URL)), nil
}

func (h *ServiceRouteBindingHandler) RegisterRoutes(router *mux.Router) {
	router.Path(ServiceRouteBindingsPath).Methods("GET").HandlerFunc(h.handlerWrapper.Wrap(h.serviceRouteBindingsListHandler))
}
