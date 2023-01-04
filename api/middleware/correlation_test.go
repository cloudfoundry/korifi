package middleware_test

import (
	"net/http"
	"strings"

	"code.cloudfoundry.org/korifi/api/middleware"
	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

func handler(w http.ResponseWriter, r *http.Request) {
	logger := logr.FromContextOrDiscard(r.Context())
	logger.Info("hello")
}

var _ = Describe("Correlation", func() {
	var (
		requestHeaders http.Header
		buf            *strings.Builder
	)

	BeforeEach(func() {
		requestHeaders = http.Header{}
		buf = &strings.Builder{}
	})

	JustBeforeEach(func() {
		request, err := http.NewRequest(http.MethodGet, "http://localhost/foo", nil)
		Expect(err).NotTo(HaveOccurred())

		request.Header = requestHeaders

		logger := zap.New(zap.WriteTo(buf))
		middleware.Correlation(logger)(http.HandlerFunc(handler)).ServeHTTP(rr, request)
	})

	It("logs with the correlation ID and returns it in a header", func() {
		Expect(rr).To(HaveHTTPHeaderWithValue("X-Correlation-Id", Not(BeEmpty())))
		corrID := rr.Header().Get("X-Correlation-Id")
		Expect(buf.String()).To(ContainSubstring("hello"))
		Expect(buf.String()).To(ContainSubstring(`"correlation-id":"` + corrID + `"`))
	})

	When("correlation ID is passed in a header", func() {
		BeforeEach(func() {
			requestHeaders.Set("X-Correlation-Id", "my-corr-id")
		})

		It("uses that ID", func() {
			Expect(rr).To(HaveHTTPHeaderWithValue("X-Correlation-Id", Equal("my-corr-id")))
			Expect(buf.String()).To(ContainSubstring(`"correlation-id":"my-corr-id"`))
		})
	})
})
