package apis_test

import (
	"net/http"
	"strings"

	. "code.cloudfoundry.org/cf-k8s-controllers/api/apis"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("ResourceMatchesHandler", func() {
	Describe("Get Resource Match Endpoint", func() {
		makePostRequest := func(body string) {
			req, err := http.NewRequest("POST", "/v3/resource_matches", strings.NewReader(body))
			Expect(err).NotTo(HaveOccurred())

			router.ServeHTTP(rr, req)
		}

		BeforeEach(func() {
			apiHandler := NewResourceMatchesHandler("foo://my-server")
			apiHandler.RegisterRoutes(router)
		})

		When("ResourceMatchesHandler is called", func() {
			BeforeEach(func() {
				makePostRequest(`{}`)
			})

			It("returns status 201 Created", func() {
				Expect(rr.Code).To(Equal(http.StatusCreated), "Matching HTTP response code:")
			})

			It("returns Content-Type as JSON in header", func() {
				contentTypeHeader := rr.Header().Get("Content-Type")
				Expect(contentTypeHeader).To(Equal(jsonHeader), "Matching Content-Type header:")
			})

			It("returns a CF API formatted Error response", func() {
				Expect(rr.Body.String()).To(MatchJSON(`{
				"resources": []
			  }`), "Response body matches response:")
			})
		})
	})
})
