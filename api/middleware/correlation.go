package middleware

import (
	"net/http"

	"github.com/go-logr/logr"
	"github.com/google/uuid"
)

const CorrelationIDHeader = "X-Correlation-ID"

func Correlation(logger logr.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id := r.Header.Get(CorrelationIDHeader)
			if id == "" {
				id = uuid.NewString()
			}

			l := logger.WithValues("correlation-id", id)
			r = r.WithContext(logr.NewContext(r.Context(), l))

			w.Header().Add(CorrelationIDHeader, id)

			next.ServeHTTP(w, r)
		})
	}
}
