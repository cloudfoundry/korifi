package apis_test

import (
	"net/http"

	"code.cloudfoundry.org/korifi/api/apis"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("LogCacheHandler", func() {
	var req *http.Request

	BeforeEach(func() {
		handler := apis.NewLogCacheHandler()
		handler.RegisterRoutes(router)
	})

	JustBeforeEach(func() {
		router.ServeHTTP(rr, req)
	})

	Describe("the GET /api/v1/info endpoint", func() {
		BeforeEach(func() {
			var err error
			req, err = http.NewRequest("GET", "/api/v1/info", nil)
			Expect(err).NotTo(HaveOccurred())
		})

		It("returns status 200 OK", func() {
			Expect(rr.Code).To(Equal(http.StatusOK))
		})

		It("returns Content-Type as JSON in header", func() {
			contentTypeHeader := rr.Header().Get("Content-Type")
			Expect(contentTypeHeader).To(Equal(jsonHeader))
		})

		It("matches the expected response body format", func() {
			expectedBody := `{"version":"2.11.4+cf-k8s","vm_uptime":"0"}`
			Expect(rr.Body).To(MatchJSON(expectedBody))
		})
	})

	Describe("the GET /api/v1/read/<app-guid> endpoint", func() {
		BeforeEach(func() {
			var err error
			req, err = http.NewRequest("GET", "/api/v1/read/unused-app-guid", nil)
			Expect(err).NotTo(HaveOccurred())
		})

		It("returns status 200 OK", func() {
			Expect(rr.Code).To(Equal(http.StatusOK))
		})

		It("returns Content-Type as JSON in header", func() {
			contentTypeHeader := rr.Header().Get("Content-Type")
			Expect(contentTypeHeader).To(Equal(jsonHeader))
		})

		It("returns a hardcoded list of empty log envelopes", func() {
			expectedBody := `{"envelopes":{"batch":[]}}`
			Expect(rr.Body).To(MatchJSON(expectedBody))
		})
	})
})
