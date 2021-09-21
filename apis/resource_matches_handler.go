package apis

import (
	"net/http"
)

type ResourceMatchesHandler struct {
	ServerURL string
}

func (h *ResourceMatchesHandler) ResourceMatchesPostHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	w.Write([]byte(`{"resources":[]}`))
}
