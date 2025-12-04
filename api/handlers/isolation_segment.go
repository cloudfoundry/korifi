package handlers

import (
	"net/http"
	"net/url"

	"code.cloudfoundry.org/korifi/api/presenter"
	"code.cloudfoundry.org/korifi/api/repositories"
	"code.cloudfoundry.org/korifi/api/routing"
)

const (
	IsolationSegmentsPath = "/v3/isolation_segments"
)

type IsolationSegment struct {
	apiBaseURL url.URL
}

func NewIsolationSegment(apiBaseURL url.URL) *IsolationSegment {
	return &IsolationSegment{
		apiBaseURL: apiBaseURL,
	}
}

func (h *IsolationSegment) list(r *http.Request) (*routing.Response, error) {
	return routing.NewResponse(http.StatusOK).WithBody(presenter.ForList(presenter.Empty, repositories.ListResult[any]{}, h.apiBaseURL, *r.URL)), nil
}

func (h *IsolationSegment) AuthenticatedRoutes() []routing.Route {
	return []routing.Route{
		{Method: "GET", Pattern: IsolationSegmentsPath, Handler: h.list},
	}
}

func (h *IsolationSegment) UnauthenticatedRoutes() []routing.Route {
	return nil
}
