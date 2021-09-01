package apis

import "net/http"

type RootHandler struct {
	ServerURL string
}

func (h *RootHandler) RootGetHandler(w http.ResponseWriter, r *http.Request) {
	body := `{"links":{"self":{"href":"` + h.ServerURL + `"},"bits_service":null,"cloud_controller_v2":null,"cloud_controller_v3":{"href":"` + h.ServerURL + `/v3","meta":{"version":"3.90.0"}},"network_policy_v0":null,"network_policy_v1":null,"login":null,"uaa":null,"credhub":null,"routing":null,"logging":null,"log_cache":null,"log_stream":null,"app_ssh":null}}`

	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(body))
}
