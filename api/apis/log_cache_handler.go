package apis

import (
	"net/http"

	"github.com/gorilla/mux"
)

const (
	LogCacheInfoEndpoint = "/api/v1/info"
	LogCacheReadEndpoint = "/api/v1/read/{guid}"
	logCacheVersion      = "2.11.4+cf-k8s"
)

// LogCacheHandler implements the minimal set of log-cache API endpoints/features necessary
// to support the "cf push" workflow.
// It does not support actually fetching and returning application logs at this time
type LogCacheHandler struct{}

func NewLogCacheHandler() *LogCacheHandler {
	return &LogCacheHandler{}
}

func (h *LogCacheHandler) logCacheInfoHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	writeStringResponse(w, http.StatusOK, `{"version":"`+logCacheVersion+`","vm_uptime":"0"}`)
}

func (h *LogCacheHandler) logCacheEmptyReadHandler(w http.ResponseWriter, r *http.Request) {
	// Since we're not currently returning an app logs there is no need to check the validity of the app guid
	// provided in the request. A full implementation of this endpoint needs to have the appropriate
	// validity and authorization checks in place.
	w.Header().Set("Content-Type", "application/json")
	writeStringResponse(w, http.StatusOK, `{"envelopes":{"batch":[]}}`)
}

func (h *LogCacheHandler) RegisterRoutes(router *mux.Router) {
	router.Path(LogCacheInfoEndpoint).Methods("GET").HandlerFunc(h.logCacheInfoHandler)
	router.Path(LogCacheReadEndpoint).Methods("GET").HandlerFunc(h.logCacheEmptyReadHandler)
}
