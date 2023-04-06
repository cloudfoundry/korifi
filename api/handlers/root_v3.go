package handlers

import (
	"net/http"
	"net/url"

	"code.cloudfoundry.org/korifi/api/presenter"
	"code.cloudfoundry.org/korifi/api/routing"
)

const (
	RootV3Path = "/v3"
)

type RootV3 struct {
	baseURL url.URL
}

func NewRootV3(baseURL url.URL) *RootV3 {
	return &RootV3{
		baseURL: baseURL,
	}
}

func (h *RootV3) get(r *http.Request) (*routing.Response, error) {
	return routing.NewResponse(http.StatusOK).WithBody(presenter.ForRootV3(h.baseURL)), nil
}

func (h *RootV3) UnauthenticatedRoutes() []routing.Route {
	return []routing.Route{
		{Method: "GET", Pattern: RootV3Path, Handler: h.get},
	}
}

func (h *RootV3) AuthenticatedRoutes() []routing.Route {
	return nil
}
