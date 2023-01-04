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

type RootHandler struct {
	serverURL string
}

func NewRootHandler(serverURL string) *RootHandler {
	return &RootHandler{
		serverURL: serverURL,
	}
}

func (h *RootHandler) rootGetHandler(r *http.Request) (*routing.Response, error) {
	return routing.NewHandlerResponse(http.StatusOK).WithBody(presenter.GetRootResponse(h.serverURL)), nil
}

func (h *RootHandler) RegisterRoutes(router *chi.Mux) {
	router.Method("GET", RootPath, routing.Handler(h.rootGetHandler))
}
