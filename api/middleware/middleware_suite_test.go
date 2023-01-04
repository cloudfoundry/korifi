package middleware_test

import (
	"net/http/httptest"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const (
	defaultServerURL = "https://api.example.org"
	jsonHeader       = "application/json"
)

var rr *httptest.ResponseRecorder

func TestApis(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Middleware Suite")
}

var _ = BeforeEach(func() {
	rr = httptest.NewRecorder()
})
