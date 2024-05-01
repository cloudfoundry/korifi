package handlers

import (
	"net/http"
	"net/url"

	"code.cloudfoundry.org/korifi/api/presenter"
	"code.cloudfoundry.org/korifi/api/routing"
)

const (
	InfoV3Path = "/v3/info"
)

type InfoV3 struct {
	baseURL url.URL
}

func NewInfoV3(baseURL url.URL) *InfoV3 {
	return &InfoV3{
		baseURL: baseURL,
	}
}

func (h *InfoV3) get(r *http.Request) (*routing.Response, error) {
	return routing.NewResponse(http.StatusOK).WithBody(presenter.ForInfoV3(h.baseURL)), nil
}

func (h *InfoV3) UnauthenticatedRoutes() []routing.Route {
	return []routing.Route{
		{Method: "GET", Pattern: InfoV3Path, Handler: h.get},
	}
}

func (h *InfoV3) AuthenticatedRoutes() []routing.Route {
	return nil
}
