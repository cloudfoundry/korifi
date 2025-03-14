package middleware_test

import (
	"net/http"

	"code.cloudfoundry.org/korifi/api/middleware"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("DisableSecurityGroups", func() {
	var securityGroupsMiddleware http.Handler

	BeforeEach(func() {
		securityGroupsMiddleware = middleware.DisableSecurityGroups(
			http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusTeapot)
			}))
	})

	It("allows requests not related to security groups", func() {
		request, err := http.NewRequest(http.MethodGet, "/v3/foo", nil)
		Expect(err).NotTo(HaveOccurred())

		securityGroupsMiddleware.ServeHTTP(rr, request)
		Expect(rr).To(HaveHTTPStatus(http.StatusTeapot))
	})

	DescribeTable("Security Groups endpoints",
		func(requestURL string) {
			request, err := http.NewRequest(http.MethodGet, requestURL, nil)
			Expect(err).NotTo(HaveOccurred())

			securityGroupsMiddleware.ServeHTTP(rr, request)
			Expect(rr).To(HaveHTTPStatus(http.StatusBadRequest))
			Expect(rr).To(HaveHTTPBody(ContainSubstring("Experimental security groups support is not enabled")))
		},
		Entry("/v3/security_groups", "/v3/security_groups"),
	)
})
