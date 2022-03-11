package apis

import (
	"net/http"

	"github.com/gorilla/mux"
)

const (
	LogCacheInfoPath = "/api/v1/info"
	LogCacheReadPath = "/api/v1/read/{guid}"
	logCacheVersion  = "2.11.4+cf-k8s"
)

// LogCacheHandler implements the minimal set of log-cache API endpoints/features necessary
// to support the "cf push" workflow.
// It does not support actually fetching and returning application logs at this time
type LogCacheHandler struct{}

func NewLogCacheHandler() *LogCacheHandler {
	return &LogCacheHandler{}
}

func (h *LogCacheHandler) logCacheInfoHandler(w http.ResponseWriter, r *http.Request) {
	writeResponse(w, http.StatusOK, map[string]interface{}{
		"version":   logCacheVersion,
		"vm_uptime": "0",
	})
}

func (h *LogCacheHandler) logCacheEmptyReadHandler(w http.ResponseWriter, r *http.Request) {
	// Since we're not currently returning an app logs there is no need to check the validity of the app guid
	// provided in the request. A full implementation of this endpoint needs to have the appropriate
	// validity and authorization checks in place.
	writeResponse(w, http.StatusOK, map[string]interface{}{
		"envelopes": map[string]interface{}{
			"batch": []interface{}{},
		},
	})
}

func (h *LogCacheHandler) RegisterRoutes(router *mux.Router) {
	router.Path(LogCacheInfoPath).Methods("GET").HandlerFunc(h.logCacheInfoHandler)
	router.Path(LogCacheReadPath).Methods("GET").HandlerFunc(h.logCacheEmptyReadHandler)
}
