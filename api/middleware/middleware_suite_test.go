package middleware_test

import (
	"net/http/httptest"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var rr *httptest.ResponseRecorder

func TestApis(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Middleware Suite")
}

var _ = BeforeEach(func() {
	rr = httptest.NewRecorder()
})
