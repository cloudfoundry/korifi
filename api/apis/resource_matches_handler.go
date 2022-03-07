package apis

import (
	"net/http"

	"code.cloudfoundry.org/cf-k8s-controllers/api/authorization"
	"github.com/go-logr/logr"
	"github.com/gorilla/mux"
)

const (
	ResourceMatchesEndpoint = "/v3/resource_matches"
)

type ResourceMatchesHandler struct {
	logger logr.Logger
}

func NewResourceMatchesHandler(logger logr.Logger) *ResourceMatchesHandler {
	return &ResourceMatchesHandler{
		logger: logger,
	}
}

func (h *ResourceMatchesHandler) resourceMatchesPostHandler(_ authorization.Info, r *http.Request) (*HandlerResponse, error) {
	return NewHandlerResponse(http.StatusCreated).WithBody(map[string]interface{}{
		"resources": []interface{}{},
	}), nil
}

func (h *ResourceMatchesHandler) RegisterRoutes(router *mux.Router) {
	w := NewAuthAwareHandlerFuncWrapper(h.logger)
	router.Path(ResourceMatchesEndpoint).Methods("POST").HandlerFunc(w.Wrap(h.resourceMatchesPostHandler))
}
