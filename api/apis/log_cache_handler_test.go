package apis_test

import (
	"net/http"

	"code.cloudfoundry.org/cf-k8s-controllers/api/apis"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("LogCacheHandler", func() {
	Describe("the GET /api/v1/info endpoint", func() {
		BeforeEach(func() {
			req, err := http.NewRequest("GET", "/api/v1/info", nil)
			Expect(err).NotTo(HaveOccurred())

			apiHandler := apis.NewLogCacheHandler()
			apiHandler.RegisterRoutes(router)

			router.ServeHTTP(rr, req)
		})

		It("returns status 200 OK", func() {
			Expect(rr.Code).To(Equal(http.StatusOK), "Matching HTTP response code:")
		})

		It("returns Content-Type as JSON in header", func() {
			contentTypeHeader := rr.Header().Get("Content-Type")
			Expect(contentTypeHeader).To(Equal(jsonHeader), "Matching Content-Type header:")
		})

		It("matches the expected response body format", func() {
			expectedBody := `{"version":"2.11.4+cf-k8s","vm_uptime":"0"}`
			Expect(rr.Body.String()).To(Equal(expectedBody), "Response body matches LogCacheHandler response:")
		})
	})

	Describe("the GET /api/v1/read/<app-guid> endpoint", func() {
		BeforeEach(func() {
			req, err := http.NewRequest("GET", "/api/v1/read/unused-app-guid", nil)
			Expect(err).NotTo(HaveOccurred())

			apiHandler := apis.NewLogCacheHandler()
			apiHandler.RegisterRoutes(router)

			router.ServeHTTP(rr, req)
		})

		It("returns status 200 OK", func() {
			Expect(rr.Code).To(Equal(http.StatusOK), "Matching HTTP response code:")
		})

		It("returns Content-Type as JSON in header", func() {
			contentTypeHeader := rr.Header().Get("Content-Type")
			Expect(contentTypeHeader).To(Equal(jsonHeader), "Matching Content-Type header:")
		})

		It("returns a hardcoded list of empty log envelopes", func() {
			expectedBody := `{"envelopes":{"batch":[]}}`
			Expect(rr.Body.String()).To(Equal(expectedBody), "Response body matches LogCacheHandler response:")
		})
	})
})
