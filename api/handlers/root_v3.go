package handlers

import (
	"net/http"

	"code.cloudfoundry.org/korifi/api/routing"
)

const (
	RootV3Path = "/v3"
)

type RootV3 struct {
	serverURL string
}

func NewRootV3(serverURL string) *RootV3 {
	return &RootV3{
		serverURL: serverURL,
	}
}

func (h *RootV3) get(r *http.Request) (*routing.Response, error) {
	return routing.NewResponse(http.StatusOK).WithBody(map[string]interface{}{
		"links": map[string]interface{}{
			"self": map[string]interface{}{
				"href": h.serverURL + "/v3",
			},
		},
	}), nil
}

func (h *RootV3) UnauthenticatedRoutes() []routing.Route {
	return []routing.Route{
		{Method: "GET", Pattern: RootV3Path, Handler: h.get},
	}
}

func (h *RootV3) AuthenticatedRoutes() []routing.Route {
	return nil
}
