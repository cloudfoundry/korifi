package broker

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2" //lint:ignore ST1001 this is a test file
	. "github.com/onsi/gomega"    //lint:ignore ST1001 this is a test file
)

type BrokerServer struct {
	httpServer *httptest.Server
	requests   []*http.Request
	handlers   map[string]http.Handler
}

func NewServer() *BrokerServer {
	return &BrokerServer{
		requests: []*http.Request{},
		handlers: map[string]http.Handler{},
	}
}

func (b *BrokerServer) WithResponse(pattern string, response map[string]any, statusCode int) *BrokerServer {
	GinkgoHelper()

	respBytes, err := json.Marshal(response)
	Expect(err).NotTo(HaveOccurred())

	return b.WithHandler(pattern, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(statusCode)
		_, err := w.Write(respBytes)
		Expect(err).NotTo(HaveOccurred())
	}))
}

func (b *BrokerServer) WithHandler(pattern string, handler http.Handler) *BrokerServer {
	b.handlers[pattern] = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		bodyBytes, err := io.ReadAll(r.Body)
		Expect(err).NotTo(HaveOccurred())

		recordedRequest := r.Clone(context.Background())
		recordedRequest.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
		b.requests = append(b.requests, recordedRequest)

		executedRequest := r.Clone(r.Context())
		executedRequest.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
		handler.ServeHTTP(w, executedRequest)
	})

	return b
}

func (b *BrokerServer) Start() *BrokerServer {
	mux := http.NewServeMux()
	for pattern, handler := range b.handlers {
		mux.Handle(pattern, handler)
	}
	b.httpServer = httptest.NewTLSServer(mux)
	return b
}

func (b *BrokerServer) URL() string {
	return b.httpServer.URL
}

func (b *BrokerServer) Stop() {
	b.httpServer.Close()
}

func (b *BrokerServer) ServedRequests() []*http.Request {
	return b.requests
}
