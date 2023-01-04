package handlers

import (
	"net/http"

	"code.cloudfoundry.org/korifi/api/routing"
	"github.com/go-chi/chi"
)

const (
	RootV3Path = "/v3"
)

type RootV3 struct {
	serverURL string
}

func NewRootV3(serverURL string) *RootV3 {
	return &RootV3{
		serverURL: serverURL,
	}
}

func (h *RootV3) get(r *http.Request) (*routing.Response, error) {
	return routing.NewResponse(http.StatusOK).WithBody(map[string]interface{}{
		"links": map[string]interface{}{
			"self": map[string]interface{}{
				"href": h.serverURL + "/v3",
			},
		},
	}), nil
}

func (h *RootV3) RegisterRoutes(router *chi.Mux) {
	router.Method("GET", RootV3Path, routing.Handler(h.get))
}
