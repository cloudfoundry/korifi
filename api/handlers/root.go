package handlers

import (
	"net/http"

	"code.cloudfoundry.org/korifi/api/presenter"
	"code.cloudfoundry.org/korifi/api/routing"
	"github.com/go-chi/chi"
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

func (h *Root) RegisterRoutes(router *chi.Mux) {
	router.Method("GET", RootPath, routing.Handler(h.get))
}
