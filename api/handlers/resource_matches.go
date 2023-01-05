package handlers

import (
	"net/http"

	"code.cloudfoundry.org/korifi/api/routing"
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

func (h *ResourceMatches) UnauthenticatedRoutes() []routing.Route {
	return nil
}

func (h *ResourceMatches) AuthenticatedRoutes() []routing.Route {
	return []routing.Route{
		{Method: "POST", Pattern: ResourceMatchesPath, Handler: h.create},
	}
}
