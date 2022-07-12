package handlers

import (
	"io"
	"net/http"
	"os"
	"time"

	"code.cloudfoundry.org/korifi/api/correlation"
	"github.com/gorilla/handlers"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

type HTTPLogging struct{}

func NewHTTPLogging() HTTPLogging {
	return HTTPLogging{}
}

func (l HTTPLogging) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t1 := time.Now()
		logger := correlation.AddCorrelationIDToLogger(r.Context(), logf.Log.WithName("http-logger"))
		logger.Info("request", "url", r.URL, "method", r.Method, "remoteAddr", r.RemoteAddr, "contentLength", r.ContentLength)

		handlers.CustomLoggingHandler(os.Stdout, next, func(writer io.Writer, params handlers.LogFormatterParams) {
			logger.Info("response", "status", params.StatusCode, "size", params.Size, "durationMillis", time.Since(t1).Milliseconds())
		}).ServeHTTP(w, r)
	})
}
