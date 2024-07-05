package broker

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"

	"code.cloudfoundry.org/korifi/controllers/controllers/services/brokers/osbapi"

	. "github.com/onsi/ginkgo/v2" //lint:ignore ST1001 this is a test file
	. "github.com/onsi/gomega"    //lint:ignore ST1001 this is a test file
)

type BrokerServer struct {
	mux        *http.ServeMux
	httpServer *httptest.Server
	requests   []*http.Request
}

func NewServer() *BrokerServer {
	return &BrokerServer{
		mux:      http.NewServeMux(),
		requests: []*http.Request{},
	}
}

func (b *BrokerServer) WithCatalog(catalog *osbapi.Catalog) *BrokerServer {
	GinkgoHelper()

	catalogBytes, err := json.Marshal(catalog)
	Expect(err).NotTo(HaveOccurred())

	return b.WithHandler("/v2/catalog", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(catalogBytes)
	}))
}

func (b *BrokerServer) WithHandler(pattern string, handler http.Handler) *BrokerServer {
	b.mux.Handle(pattern, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b.requests = append(b.requests, r)
		handler.ServeHTTP(w, r)
	}))

	return b
}

func (b *BrokerServer) Start() *BrokerServer {
	b.httpServer = httptest.NewTLSServer(b.mux)
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
