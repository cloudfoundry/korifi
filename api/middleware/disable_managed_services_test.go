package middleware_test

import (
	"net/http"

	"code.cloudfoundry.org/korifi/api/middleware"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("DisableManagedServices", func() {
	var managedServicesMiddleware http.Handler

	BeforeEach(func() {
		managedServicesMiddleware = middleware.DisableManagedServices(
			http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusTeapot)
			}))
	})

	It("allows requests not related to managed services", func() {
		request, err := http.NewRequest(http.MethodGet, "/v3/foo", nil)
		Expect(err).NotTo(HaveOccurred())

		managedServicesMiddleware.ServeHTTP(rr, request)
		Expect(rr).To(HaveHTTPStatus(http.StatusTeapot))
	})

	DescribeTable("Managed services endpoints",
		func(requestURL string) {
			request, err := http.NewRequest(http.MethodGet, requestURL, nil)
			Expect(err).NotTo(HaveOccurred())

			managedServicesMiddleware.ServeHTTP(rr, request)
			Expect(rr).To(HaveHTTPStatus(http.StatusBadRequest))
			Expect(rr).To(HaveHTTPBody(ContainSubstring("Experimental managed services support is not enabled")))
		},
		Entry("/v3/service_brokers", "/v3/service_brokers"),
		Entry("/v3/service_brokers/123", "/v3/service_brokers/123"),
		Entry("/v3/service_offerings", "/v3/service_offerings"),
		Entry("/v3/service_offerings/123", "/v3/service_offerings/123"),
		Entry("/v3/service_plans", "/v3/service_plans"),
		Entry("/v3/service_plans/123", "/v3/service_plans/123"),
		Entry("/v3/service_plans/123/visibility", "/v3/service_plans/123/visibility"),
	)
})
