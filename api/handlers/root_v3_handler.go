package handlers

import (
	"context"
	"net/http"

	"code.cloudfoundry.org/korifi/api/authorization"
	"github.com/go-chi/chi"
	"github.com/go-logr/logr"
	ctrl "sigs.k8s.io/controller-runtime"
)

const (
	RootV3Path = "/v3"
)

type RootV3Handler struct {
	serverURL                     string
	unauthenticatedHandlerWrapper *AuthAwareHandlerFuncWrapper
}

func NewRootV3Handler(serverURL string) *RootV3Handler {
	return &RootV3Handler{
		serverURL:                     serverURL,
		unauthenticatedHandlerWrapper: NewUnauthenticatedHandlerFuncWrapper(ctrl.Log.WithName("RootHandler")),
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

func (h *RootV3Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	router := chi.NewRouter()
	router.Get(RootV3Path, h.unauthenticatedHandlerWrapper.Wrap(h.rootV3GetHandler))
	router.ServeHTTP(w, r)
}
