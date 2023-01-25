package handlers_test

import (
	"net/http"

	"code.cloudfoundry.org/korifi/api/handlers"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("User", func() {
	var query string

	BeforeEach(func() {
		query = ""
		userHandler := handlers.NewUser(*serverURL)
		routerBuilder.LoadRoutes(userHandler)
	})

	JustBeforeEach(func() {
		req, err := http.NewRequestWithContext(ctx, "GET", "/v3/users"+query, nil)
		Expect(err).NotTo(HaveOccurred())
		routerBuilder.Build().ServeHTTP(rr, req)
	})

	Describe("GET /v3/users", func() {
		When("no parameters are provided", func() {
			It("returns an empty list", func() {
				Expect(rr).To(HaveHTTPStatus(http.StatusOK))
				Expect(rr).To(HaveHTTPHeaderWithValue("Content-Type", "application/json"))
				Expect(rr).To(HaveHTTPBody(MatchJSON(`{
					"pagination": {
						"total_results": 0,
						"total_pages": 1,
						"first": {
							"href": "https://api.example.org/v3/users"
						},
						"last": {
							"href": "https://api.example.org/v3/users"
						},
						"next": null,
						"previous": null
					},
					"resources": []
				}`)))
			})
		})

		When("usernames are passed", func() {
			BeforeEach(func() {
				query = "?usernames=foo,bar"
			})

			It("returns a list of users matching the usernames", func() {
				Expect(rr).To(HaveHTTPStatus(http.StatusOK))
				Expect(rr).To(HaveHTTPHeaderWithValue("Content-Type", "application/json"))
				Expect(rr).To(HaveHTTPBody(MatchJSON(`{
					"pagination": {
						"total_results": 2,
						"total_pages": 1,
						"first": {
							"href": "https://api.example.org/v3/users?usernames=foo,bar"
						},
						"last": {
							"href": "https://api.example.org/v3/users?usernames=foo,bar"
						},
						"next": null,
						"previous": null
					},
					"resources": [
						{
							"username": "foo",
							"guid": "foo"
						},
						{
							"username": "bar",
							"guid": "bar"
						}
					]
				}`)))
			})
		})
	})
})
