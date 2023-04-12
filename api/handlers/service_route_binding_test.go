package handlers_test

import (
	"net/http"

	"code.cloudfoundry.org/korifi/api/handlers"
	. "code.cloudfoundry.org/korifi/tests/matchers"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("ServiceRouteBinding", func() {
	Describe("the GET /v3/service_route_binding endpoint", func() {
		BeforeEach(func() {
			req, err := http.NewRequestWithContext(ctx, "GET", "/v3/service_route_bindings", nil)
			Expect(err).NotTo(HaveOccurred())

			apiHandler := handlers.NewServiceRouteBinding(
				*serverURL)

			routerBuilder.LoadRoutes(apiHandler)
			routerBuilder.Build().ServeHTTP(rr, req)
		})

		It("returns an empty list", func() {
			Expect(rr).To(HaveHTTPStatus(http.StatusOK))
			Expect(rr).To(HaveHTTPHeaderWithValue("Content-Type", "application/json"))
			Expect(rr).To(HaveHTTPBody(SatisfyAll(
				MatchJSONPath("$.pagination.total_results", BeZero()),
				MatchJSONPath("$.pagination.first.href", "https://api.example.org/v3/service_route_bindings"),
			)))
		})
	})
})
