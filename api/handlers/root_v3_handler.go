package handlers

import (
	"context"
	"net/http"

	"code.cloudfoundry.org/korifi/api/authorization"
	"github.com/go-logr/logr"
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

func (h *RootV3Handler) rootV3GetHandler(ctx context.Context, logger logr.Logger, authInfo authorization.Info, r *http.Request) (*HandlerResponse, error) {
	return NewHandlerResponse(http.StatusOK).WithBody(map[string]interface{}{
		"links": map[string]interface{}{
			"self": map[string]interface{}{
				"href": h.serverURL + "/v3",
			},
		},
	}), nil
}

func (h *RootV3Handler) AuthenticatedRoutes() []Route {
	return []Route{}
}

func (h *RootV3Handler) UnauthenticatedRoutes() []Route {
	return []Route{
		{Method: "GET", Pattern: RootV3Path, HandlerFunc: h.rootV3GetHandler},
	}
}
