package apis

import (
	"net/http"

	"github.com/gorilla/mux"
)

const (
	RootGetEndpoint = "/"
)

type RootHandler struct {
	serverURL string
}

func NewRootHandler(serverURL string) *RootHandler {
	return &RootHandler{serverURL: serverURL}
}

func (h *RootHandler) rootGetHandler(w http.ResponseWriter, r *http.Request) {
	body := `{"links":{"self":{"href":"` + h.serverURL + `"},"bits_service":null,"cloud_controller_v2":null,"cloud_controller_v3":{"href":"` + h.serverURL + `/v3","meta":{"version":"3.90.0"}},"network_policy_v0":null,"network_policy_v1":null,"login":null,"uaa":null,"credhub":null,"routing":null,"logging":null,"log_cache":null,"log_stream":null,"app_ssh":null}}`

	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(body))
}

func (h *RootHandler) RegisterRoutes(router *mux.Router) {
	router.Path(RootGetEndpoint).Methods("GET").HandlerFunc(h.rootGetHandler)
}
