package routes

import (
	"github.com/gorilla/mux"
	"net/http"
)


// Just contains the CF API Routes and maps them to handler functions

const (
	RootGetEndpoint = "/"
	RootV3GetEndpoint = "/v3"
)

type httpHandlerFunction func(w http.ResponseWriter, r *http.Request)

type APIRoutes struct {
	RootV3Handler httpHandlerFunction
	RootHandler httpHandlerFunction
}

func (a *APIRoutes) RegisterRoutes( router *mux.Router ) {
	if a.RootV3Handler == nil || a.RootHandler == nil {
		panic("APIRoutes: handler was nil")
	}
	router.HandleFunc(RootGetEndpoint, a.RootHandler).Methods("GET")
	router.HandleFunc(RootV3GetEndpoint, a.RootV3Handler).Methods("GET")
}