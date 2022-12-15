package handlers

import (
	"context"
	"net/http"

	"code.cloudfoundry.org/korifi/api/authorization"
	"code.cloudfoundry.org/korifi/api/presenter"
	"github.com/go-chi/chi"
	"github.com/go-logr/logr"
	ctrl "sigs.k8s.io/controller-runtime"
)

const (
	RootPath = "/"
)

type RootHandler struct {
	serverURL                     string
	unauthenticatedHandlerWrapper *AuthAwareHandlerFuncWrapper
}

func NewRootHandler(serverURL string) *RootHandler {
	return &RootHandler{
		serverURL:                     serverURL,
		unauthenticatedHandlerWrapper: NewUnauthenticatedHandlerFuncWrapper(ctrl.Log.WithName("RootHandler")),
	}
}

func (h *RootHandler) rootGetHandler(ctx context.Context, logger logr.Logger, _ authorization.Info, r *http.Request) (*HandlerResponse, error) {
	return NewHandlerResponse(http.StatusOK).WithBody(presenter.GetRootResponse(h.serverURL)), nil
}

func (h *RootHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	router := chi.NewRouter()
	router.Get(RootPath, h.unauthenticatedHandlerWrapper.Wrap(h.rootGetHandler))
	router.ServeHTTP(w, r)
}
