package handlers_test

import (
	"net/http"
	"strings"

	. "code.cloudfoundry.org/korifi/api/handlers"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("ResourceMatches", func() {
	var req *http.Request

	BeforeEach(func() {
		apiHandler := NewResourceMatches()
		routerBuilder.LoadRoutes(apiHandler)
	})

	JustBeforeEach(func() {
		routerBuilder.Build().ServeHTTP(rr, req)
	})

	Describe("Get Resource Match Endpoint", func() {
		BeforeEach(func() {
			var err error
			req, err = http.NewRequestWithContext(ctx, "POST", "/v3/resource_matches", strings.NewReader("{}"))
			Expect(err).NotTo(HaveOccurred())
		})

		It("returns an empty list", func() {
			Expect(rr).To(HaveHTTPStatus(http.StatusCreated))
			Expect(rr).To(HaveHTTPHeaderWithValue("Content-Type", "application/json"))
			Expect(rr).To(HaveHTTPBody(MatchJSON(`{
				"resources": []
			  }`)))
		})
	})
})
