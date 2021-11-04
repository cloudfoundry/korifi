package apis

import (
	"encoding/json"
	"net/http"

	"code.cloudfoundry.org/cf-k8s-controllers/api/presenter"
	"github.com/go-logr/logr"
	"github.com/gorilla/mux"
)

const (
	RootGetEndpoint = "/"
)

type RootHandler struct {
	logger    logr.Logger
	serverURL string
}

func NewRootHandler(logger logr.Logger, serverURL string) *RootHandler {
	return &RootHandler{serverURL: serverURL}
}

func (h *RootHandler) rootGetHandler(w http.ResponseWriter, r *http.Request) {
	body, err := json.Marshal(presenter.GetRootResponse(h.serverURL))
	if err != nil {
		h.logger.Error(err, "Failed to render response")
		writeUnknownErrorResponse(w)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(body))
}

func (h *RootHandler) RegisterRoutes(router *mux.Router) {
	router.Path(RootGetEndpoint).Methods("GET").HandlerFunc(h.rootGetHandler)
}
