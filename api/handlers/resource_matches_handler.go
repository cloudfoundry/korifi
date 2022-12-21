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
	ResourceMatchesPath = "/v3/resource_matches"
)

type ResourceMatchesHandler struct {
	handlerWrapper *AuthAwareHandlerFuncWrapper
}

func NewResourceMatchesHandler() *ResourceMatchesHandler {
	return &ResourceMatchesHandler{
		handlerWrapper: NewAuthAwareHandlerFuncWrapper(ctrl.Log.WithName("ResourceMatchesHandler")),
	}
}

func (h *ResourceMatchesHandler) resourceMatchesPostHandler(ctx context.Context, logger logr.Logger, authInfo authorization.Info, r *http.Request) (*HandlerResponse, error) {
	return NewHandlerResponse(http.StatusCreated).WithBody(map[string]interface{}{
		"resources": []interface{}{},
	}), nil
}

func (h *ResourceMatchesHandler) RegisterRoutes(router *chi.Mux) {
	router.Post(ResourceMatchesPath, h.handlerWrapper.Wrap(h.resourceMatchesPostHandler))
}
