package handlers

import (
	"context"
	"net/http"

	"code.cloudfoundry.org/korifi/api/authorization"
	"code.cloudfoundry.org/korifi/api/presenter"
	"github.com/go-logr/logr"
)

const (
	RootPath = "/"
)

type RootHandler struct {
	serverURL string
}

func NewRootHandler(serverURL string) *RootHandler {
	return &RootHandler{
		serverURL: serverURL,
	}
}

func (h *RootHandler) rootGetHandler(ctx context.Context, logger logr.Logger, _ authorization.Info, r *http.Request) (*HandlerResponse, error) {
	return NewHandlerResponse(http.StatusOK).WithBody(presenter.GetRootResponse(h.serverURL)), nil
}

func (h *RootHandler) AuthenticatedRoutes() []Route {
	return []Route{}
}

func (h *RootHandler) UnauthenticatedRoutes() []Route {
	return []Route{
		{Method: "GET", Pattern: RootPath, HandlerFunc: h.rootGetHandler},
	}
}
