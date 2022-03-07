package apis

import (
	"net/http"

	"github.com/gorilla/mux"
)

const (
	RootV3GetEndpoint = "/v3"
)

type RootV3Handler struct {
	serverURL string
}

func NewRootV3Handler(serverURL string) *RootV3Handler {
	return &RootV3Handler{serverURL: serverURL}
}

func (h *RootV3Handler) rootV3GetHandler(w http.ResponseWriter, r *http.Request) {
	writeResponse(w, http.StatusOK, map[string]interface{}{
		"links": map[string]interface{}{
			"self": map[string]interface{}{
				"href": h.serverURL + "/v3",
			},
		},
	})
}

func (h *RootV3Handler) RegisterRoutes(router *mux.Router) {
	router.Path(RootV3GetEndpoint).Methods("GET").HandlerFunc(h.rootV3GetHandler)
}
