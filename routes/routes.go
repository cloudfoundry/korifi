package routes

import (
	"net/http"

	"github.com/gorilla/mux"
)

// Just contains the CF API Routes and maps them to handler functions

const (
	RootGetEndpoint       = "/"
	RootV3GetEndpoint     = "/v3"
	AppCreateEndpoint     = "/v3/apps"
	AppGetEndpoint        = "/v3/apps/{guid}"
	RouteGetEndpoint      = "/v3/routes/{guid}"
	PackageCreateEndpoint = "/v3/packages"
)

type httpHandlerFunction func(w http.ResponseWriter, r *http.Request)

type APIRoutes struct {
	RootV3Handler        httpHandlerFunction
	RootHandler          httpHandlerFunction
	AppCreateHandler     httpHandlerFunction
	AppGetHandler        httpHandlerFunction
	RouteGetHandler      httpHandlerFunction
	PackageCreateHandler httpHandlerFunction
}

func (a *APIRoutes) RegisterRoutes(router *mux.Router) {
	// Is this a useful check?
	if a.RootV3Handler == nil ||
		a.RootHandler == nil ||
		a.AppGetHandler == nil ||
		a.AppCreateHandler == nil ||
		a.RouteGetHandler == nil ||
		a.PackageCreateHandler == nil {
		panic("APIRoutes: handler was nil")
	}

	router.HandleFunc(RootGetEndpoint, a.RootHandler).Methods("GET")
	router.HandleFunc(RootV3GetEndpoint, a.RootV3Handler).Methods("GET")
	router.HandleFunc(AppCreateEndpoint, a.AppCreateHandler).Methods("POST")
	router.HandleFunc(AppGetEndpoint, a.AppGetHandler).Methods("GET")
	router.HandleFunc(RouteGetEndpoint, a.RouteGetHandler).Methods("GET")
	router.HandleFunc(PackageCreateEndpoint, a.PackageCreateHandler).Methods("POST")
}
