package routes

import (
	"net/http"

	"github.com/gorilla/mux"
)

// Just contains the CF API Routes and maps them to handler functions

const (
	RootGetEndpoint    = "/"
	RootV3GetEndpoint  = "/v3"
	AppGetEndpoint     = "/v3/apps/{guid}"
	AppsCreateEndpoint = RootV3GetEndpoint + "/apps"
	RouteGetEndpoint   = "/v3/routes/{guid}"
)

type httpHandlerFunction func(w http.ResponseWriter, r *http.Request)

type APIRoutes struct {
	RootV3Handler     httpHandlerFunction
	RootHandler       httpHandlerFunction
	AppHandler        httpHandlerFunction
	RouteHandler      httpHandlerFunction
	AppsCreateHandler httpHandlerFunction
}

func (a *APIRoutes) RegisterRoutes(router *mux.Router) {
	// Is this a useful check?
	if a.RootV3Handler == nil || a.RootHandler == nil || a.AppHandler == nil || a.RouteHandler == nil || a.AppsCreateHandler == nil {
		panic("APIRoutes: handler was nil")
	}
	router.HandleFunc(RootGetEndpoint, a.RootHandler).Methods("GET")
	router.HandleFunc(RootV3GetEndpoint, a.RootV3Handler).Methods("GET")
	router.HandleFunc(AppsCreateEndpoint, a.AppsCreateHandler).Methods("POST")
	router.HandleFunc(AppGetEndpoint, a.AppHandler).Methods("GET")
	router.HandleFunc(RouteGetEndpoint, a.RouteHandler).Methods("GET")
}
