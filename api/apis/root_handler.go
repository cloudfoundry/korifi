package apis

import (
	"net/http"

	"code.cloudfoundry.org/cf-k8s-controllers/api/presenter"
	"github.com/gorilla/mux"
)

const (
	RootPath = "/"
)

type RootHandler struct {
	serverURL string
}

func NewRootHandler(serverURL string) *RootHandler {
	return &RootHandler{serverURL: serverURL}
}

func (h *RootHandler) rootGetHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	writeResponse(w, http.StatusOK, presenter.GetRootResponse(h.serverURL))
}

func (h *RootHandler) RegisterRoutes(router *mux.Router) {
	router.Path(RootPath).Methods("GET").HandlerFunc(h.rootGetHandler)
}
