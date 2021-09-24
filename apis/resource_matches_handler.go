package apis

import (
	"net/http"

	"github.com/gorilla/mux"
)

const (
	ResourceMatchesEndpoint = "/v3/resource_matches"
)

type ResourceMatchesHandler struct {
	serverURL string
}

func NewResourceMatchesHandler(serverURL string) *ResourceMatchesHandler {
	return &ResourceMatchesHandler{serverURL: serverURL}
}

func (h *ResourceMatchesHandler) resourceMatchesPostHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	w.Write([]byte(`{"resources":[]}`))
}

func (h *ResourceMatchesHandler) RegisterRoutes(router *mux.Router) {
	router.Path(ResourceMatchesEndpoint).Methods("POST").HandlerFunc(h.resourceMatchesPostHandler)
}
