package handlers

import (
	"context"
	"net/http"

	"code.cloudfoundry.org/korifi/api/authorization"
	"github.com/go-logr/logr"
)

const (
	ResourceMatchesPath = "/v3/resource_matches"
)

type ResourceMatchesHandler struct{}

func NewResourceMatchesHandler() *ResourceMatchesHandler {
	return &ResourceMatchesHandler{}
}

func (h *ResourceMatchesHandler) resourceMatchesPostHandler(ctx context.Context, logger logr.Logger, authInfo authorization.Info, r *http.Request) (*HandlerResponse, error) {
	return NewHandlerResponse(http.StatusCreated).WithBody(map[string]interface{}{
		"resources": []interface{}{},
	}), nil
}

func (h *ResourceMatchesHandler) AuthenticatedRoutes() []AuthRoute {
	return []AuthRoute{
		{Method: "POST", Pattern: ResourceMatchesPath, HandlerFunc: h.resourceMatchesPostHandler},
	}
}

func (h *ResourceMatchesHandler) UnauthenticatedRoutes() []Route {
	return []Route{}
}
