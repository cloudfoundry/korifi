package apis

import (
	"net/http"

	"code.cloudfoundry.org/korifi/api/correlation"
	"github.com/google/uuid"
)

const CorrelationIDHeader = "X-Correlation-ID"

type CorrelationIDMiddleware struct{}

func NewCorrelationIDMiddleware() CorrelationIDMiddleware {
	return CorrelationIDMiddleware{}
}

func (a CorrelationIDMiddleware) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get(CorrelationIDHeader)
		if id == "" {
			id = uuid.NewString()
		}

		r = r.WithContext(correlation.ContextWithId(r.Context(), id))

		w.Header().Add(CorrelationIDHeader, id)

		next.ServeHTTP(w, r)
	})
}
