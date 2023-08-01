package handlers_test

import (
	"net/http"

	"code.cloudfoundry.org/korifi/api/handlers"
	. "code.cloudfoundry.org/korifi/tests/matchers"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("RootV3", func() {
	var req *http.Request

	BeforeEach(func() {
		apiHandler := handlers.NewRootV3(*serverURL)
		routerBuilder.LoadRoutes(apiHandler)
	})

	JustBeforeEach(func() {
		routerBuilder.Build().ServeHTTP(rr, req)
	})

	Describe("the GET /v3 endpoint", func() {
		BeforeEach(func() {
			var err error
			req, err = http.NewRequest("GET", "/v3", nil)
			Expect(err).NotTo(HaveOccurred())
		})

		It("returns the expected response", func() {
			Expect(rr).To(HaveHTTPStatus(http.StatusOK))
			Expect(rr).To(HaveHTTPHeaderWithValue("Content-Type", "application/json"))

			Expect(rr).To(HaveHTTPBody(SatisfyAll(
				MatchJSONPath("$.links.self.href", "https://api.example.org/v3"),
			)))
		})
	})
})
