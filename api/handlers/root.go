package handlers

import (
	"net/http"

	"code.cloudfoundry.org/korifi/api/presenter"
	"code.cloudfoundry.org/korifi/api/routing"
)

const (
	RootPath = "/"
)

type Root struct {
	serverURL string
}

func NewRoot(serverURL string) *Root {
	return &Root{
		serverURL: serverURL,
	}
}

func (h *Root) get(r *http.Request) (*routing.Response, error) {
	return routing.NewResponse(http.StatusOK).WithBody(presenter.GetRootResponse(h.serverURL)), nil
}

func (h *Root) UnauthenticatedRoutes() []routing.Route {
	return []routing.Route{
		{Method: "GET", Pattern: RootPath, Handler: h.get},
	}
}

func (h *Root) AuthenticatedRoutes() []routing.Route {
	return nil
}
