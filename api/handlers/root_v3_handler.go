package handlers

import (
	"net/http"

	"code.cloudfoundry.org/korifi/api/routing"
	"github.com/go-chi/chi"
)

const (
	RootV3Path = "/v3"
)

type RootV3Handler struct {
	serverURL string
}

func NewRootV3Handler(serverURL string) *RootV3Handler {
	return &RootV3Handler{
		serverURL: serverURL,
	}
}

func (h *RootV3Handler) rootV3GetHandler(r *http.Request) (*routing.Response, error) {
	return routing.NewHandlerResponse(http.StatusOK).WithBody(map[string]interface{}{
		"links": map[string]interface{}{
			"self": map[string]interface{}{
				"href": h.serverURL + "/v3",
			},
		},
	}), nil
}

func (h *RootV3Handler) RegisterRoutes(router *chi.Mux) {
	router.Method("GET", RootV3Path, routing.Handler(h.rootV3GetHandler))
}
