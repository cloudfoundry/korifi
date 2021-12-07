package apis_test

import (
	"fmt"
	"net/http"

	logf "sigs.k8s.io/controller-runtime/pkg/log"

	"code.cloudfoundry.org/cf-k8s-controllers/api/apis"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("ServiceRouteBinding", func() {
	Describe("the GET /v3/service_route_binding endpoint", func() {
		BeforeEach(func() {
			req, err := http.NewRequest("GET", "/v3/service_route_bindings", nil)
			Expect(err).NotTo(HaveOccurred())

			apiHandler := apis.NewServiceRouteBindingHandler(
				logf.Log.WithName("ServiceRouteBinding"),
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
			}`, defaultServerURL)), "Response body matches LogCacheHandler response:")
		})
	})
})
