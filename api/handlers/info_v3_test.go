package handlers_test

import (
	"net/http"

	"code.cloudfoundry.org/korifi/api/config"
	"code.cloudfoundry.org/korifi/api/handlers"
	. "code.cloudfoundry.org/korifi/tests/matchers"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("InfoV3", func() {
	var (
		req        *http.Request
		infoConfig config.InfoConfig
	)

	BeforeEach(func() {
		apiHandler := handlers.NewInfoV3(
			*serverURL,
			infoConfig,
		)
		routerBuilder.LoadRoutes(apiHandler)
	})

	JustBeforeEach(func() {
		routerBuilder.Build().ServeHTTP(rr, req)
	})

	Describe("the GET /v3/info endpoint", func() {
		BeforeEach(func() {
			var err error
			req, err = http.NewRequest("GET", "/v3/info", nil)
			Expect(err).NotTo(HaveOccurred())
		})

		It("returns the expected response", func() {
			Expect(rr).To(HaveHTTPStatus(http.StatusOK))
			Expect(rr).To(HaveHTTPHeaderWithValue("Content-Type", "application/json"))

			Expect(rr).To(HaveHTTPBody(SatisfyAll(
				MatchJSONPath("$.links.self.href", "https://api.example.org/v3/info"),
			)))
		})
	})
})
