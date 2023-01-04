package handlers

import (
	"net/http"

	"code.cloudfoundry.org/korifi/api/routing"
	"github.com/go-chi/chi"
)

const (
	ResourceMatchesPath = "/v3/resource_matches"
)

type ResourceMatches struct{}

func NewResourceMatches() *ResourceMatches {
	return &ResourceMatches{}
}

func (h *ResourceMatches) create(r *http.Request) (*routing.Response, error) {
	return routing.NewResponse(http.StatusCreated).WithBody(map[string]interface{}{
		"resources": []interface{}{},
	}), nil
}

func (h *ResourceMatches) RegisterRoutes(router *chi.Mux) {
	router.Method("POST", ResourceMatchesPath, routing.Handler(h.create))
}
