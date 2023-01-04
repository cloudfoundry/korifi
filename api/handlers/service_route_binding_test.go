package handlers_test

import (
	"fmt"
	"net/http"

	"code.cloudfoundry.org/korifi/api/handlers"

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
			Expect(rr.Body.String()).To(MatchJSON(fmt.Sprintf(`{
				"pagination": {
				  "total_results": 0,
				  "total_pages": 1,
				  "first": {
					"href": "%[1]s/v3/service_route_bindings"
				  },
				  "last": {
					"href": "%[1]s/v3/service_route_bindings"
				  },
				  "next": null,
				  "previous": null
				},
				"resources": []
			}`, defaultServerURL)))
		})
	})
})
