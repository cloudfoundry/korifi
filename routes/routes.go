package routes

import (
	"net/http"

	"github.com/gorilla/mux"
)

// Just contains the CF API Routes and maps them to handler functions

const (
	RootGetEndpoint   = "/"
	RootV3GetEndpoint = "/v3"
	AppsGetEndpoint   = "/v3/apps/{guid}"
	RouteGetEndpoint  = "/v3/routes/{guid}"
)

type httpHandlerFunction func(w http.ResponseWriter, r *http.Request)

type APIRoutes struct {
	RootV3Handler httpHandlerFunction
	RootHandler   httpHandlerFunction
	AppsHandler   httpHandlerFunction
	RouteHandler  httpHandlerFunction
}

func (a *APIRoutes) RegisterRoutes(router *mux.Router) {
	// Is this a useful check?
	if a.RootV3Handler == nil || a.RootHandler == nil || a.AppsHandler == nil {
		panic("APIRoutes: handler was nil")
	}
	router.HandleFunc(RootGetEndpoint, a.RootHandler).Methods("GET")
	router.HandleFunc(RootV3GetEndpoint, a.RootV3Handler).Methods("GET")
	router.HandleFunc(AppsGetEndpoint, a.AppsHandler).Methods("GET")
	router.HandleFunc(RouteGetEndpoint, a.RouteHandler).Methods("GET")
}
