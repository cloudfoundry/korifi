package handlers_test

import (
	"net/http"

	"code.cloudfoundry.org/korifi/api/handlers"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("RootV3", func() {
	var req *http.Request

	BeforeEach(func() {
		apiHandler := handlers.NewRootV3(defaultServerURL)
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

		It("returns status 200 OK", func() {
			Expect(rr).To(HaveHTTPStatus(http.StatusOK))
		})

		It("returns Content-Type as JSON in header", func() {
			Expect(rr).To(HaveHTTPHeaderWithValue("Content-Type", "application/json"))
		})

		It("matches the expected response body format", func() {
			expectedBody := `{"links":{"self":{"href":"` + defaultServerURL + `/v3"}}}`
			Expect(rr).To(HaveHTTPBody(MatchJSON(expectedBody)))
		})
	})
})
