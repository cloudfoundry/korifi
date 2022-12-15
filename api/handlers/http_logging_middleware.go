package handlers

import (
	"net/http"
	"time"

	"code.cloudfoundry.org/korifi/api/correlation"
	"github.com/go-logr/logr"
)

type responseWriterWrapper struct {
	writer http.ResponseWriter
	size   int
	status int
}

func (w *responseWriterWrapper) Header() http.Header {
	return w.writer.Header()
}

func (w *responseWriterWrapper) Write(bytes []byte) (int, error) {
	size, err := w.writer.Write(bytes)
	w.size += size
	return size, err
}

func (w *responseWriterWrapper) WriteHeader(statusCode int) {
	w.writer.WriteHeader(statusCode)
	w.status = statusCode
}

type HTTPLogging struct {
	logger logr.Logger
}

func NewHTTPLogging(logger logr.Logger) HTTPLogging {
	return HTTPLogging{logger}
}

func (l HTTPLogging) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t1 := time.Now()
		logger := correlation.AddCorrelationIDToLogger(r.Context(), l.logger)

		logger.Info("request", "url", r.URL, "method", r.Method, "remoteAddr", r.RemoteAddr, "contentLength", r.ContentLength)

		wrapper := &responseWriterWrapper{writer: w}
		next.ServeHTTP(wrapper, r)
		logger.Info("response", "status", wrapper.status, "size", wrapper.size, "durationMillis", time.Since(t1).Milliseconds())
	})
}
