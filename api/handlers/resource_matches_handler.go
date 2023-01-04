package handlers

import (
	"net/http"

	"code.cloudfoundry.org/korifi/api/routing"
	"github.com/go-chi/chi"
)

const (
	ResourceMatchesPath = "/v3/resource_matches"
)

type ResourceMatchesHandler struct{}

func NewResourceMatchesHandler() *ResourceMatchesHandler {
	return &ResourceMatchesHandler{}
}

func (h *ResourceMatchesHandler) resourceMatchesPostHandler(r *http.Request) (*routing.Response, error) {
	return routing.NewHandlerResponse(http.StatusCreated).WithBody(map[string]interface{}{
		"resources": []interface{}{},
	}), nil
}

func (h *ResourceMatchesHandler) RegisterRoutes(router *chi.Mux) {
	router.Method("POST", ResourceMatchesPath, routing.Handler(h.resourceMatchesPostHandler))
}
