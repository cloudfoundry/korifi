package apis

import (
	"net/http"

	"github.com/gorilla/mux"
)

const (
	RootV3GetEndpoint = "/v3"
)

type RootV3Handler struct {
	ServerURL string
}

func (h *RootV3Handler) RootV3GetHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"links":{"self":{"href":"` + h.ServerURL + `/v3"}}}`))
}

func (h *RootV3Handler) RegisterRoutes(router *mux.Router) {
	router.Path(RootV3GetEndpoint).Methods("GET").HandlerFunc(h.RootV3GetHandler)
}
