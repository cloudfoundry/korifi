package handlers_test

import (
	"fmt"
	"net/http"

	. "code.cloudfoundry.org/korifi/api/handlers"
	"code.cloudfoundry.org/korifi/api/handlers/fake"
	"code.cloudfoundry.org/korifi/api/repositories"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Buildpack", func() {
	var (
		buildpackRepo *fake.BuildpackRepository
		req           *http.Request
	)

	BeforeEach(func() {
		buildpackRepo = new(fake.BuildpackRepository)

		apiHandler := NewBuildpack(*serverURL, buildpackRepo)
		routerBuilder.LoadRoutes(apiHandler)
	})

	JustBeforeEach(func() {
		routerBuilder.Build().ServeHTTP(rr, req)
	})

	Describe("the GET /v3/buildpacks endpoint", func() {
		BeforeEach(func() {
			buildpackRepo.ListBuildpacksReturns([]repositories.BuildpackRecord{
				{
					Name:      "paketo-foopacks/bar",
					Position:  1,
					Stack:     "waffle-house",
					Version:   "1.0.0",
					CreatedAt: "2016-03-18T23:26:46Z",
					UpdatedAt: "2016-10-17T20:00:42Z",
				},
			}, nil)
		})

		When("on the happy path", func() {
			BeforeEach(func() {
				var err error
				req, err = http.NewRequestWithContext(ctx, "GET", "/v3/buildpacks", nil)
				Expect(err).NotTo(HaveOccurred())
			})

			It("returns status 200 OK", func() {
				Expect(rr.Code).To(Equal(http.StatusOK), "Matching HTTP response code:")
			})

			It("passes authInfo from context to GetApp", func() {
				Expect(buildpackRepo.ListBuildpacksCallCount()).To(Equal(1))
				_, actualAuthInfo := buildpackRepo.ListBuildpacksArgsForCall(0)
				Expect(actualAuthInfo).To(Equal(authInfo))
			})

			It("returns the buildpacks for the default builder", func() {
				contentTypeHeader := rr.Header().Get("Content-Type")
				Expect(contentTypeHeader).To(Equal(jsonHeader), "Matching Content-Type header:")

				Expect(rr.Body.String()).To(MatchJSON(fmt.Sprintf(`{
					"pagination": {
						"total_results": 1,
						"total_pages": 1,
						"first": {
							"href": "%[1]s/v3/buildpacks"
						},
						"last": {
							"href": "%[1]s/v3/buildpacks"
						},
						"next": null,
						"previous": null
					},
					"resources": [
						{
							"guid": "",
							"created_at": "2016-03-18T23:26:46Z",
							"updated_at": "2016-10-17T20:00:42Z",
							"name": "paketo-foopacks/bar",
							"filename": "paketo-foopacks/bar@1.0.0",
							"stack": "waffle-house",
							"position": 1,
							"enabled": true,
							"locked": false,
							"metadata": {
								"labels": {},
								"annotations": {}
							},
							"links": {}
						}
					]
				}`, defaultServerURL)))
			})
		})

		When("query Parameters are provided", func() {
			BeforeEach(func() {
				var err error
				req, err = http.NewRequestWithContext(ctx, "GET", "/v3/buildpacks?order_by=position", nil)
				Expect(err).NotTo(HaveOccurred())
			})

			It("returns status 200 OK", func() {
				Expect(rr.Code).Should(Equal(http.StatusOK), "Matching HTTP response code:")
			})
		})

		When("invalid query parameters are provided", func() {
			BeforeEach(func() {
				var err error
				req, err = http.NewRequestWithContext(ctx, "GET", "/v3/buildpacks?foo=bar", nil)
				Expect(err).NotTo(HaveOccurred())
			})

			It("returns an Unknown key error", func() {
				expectUnknownKeyError("The query parameter is invalid: Valid parameters are: 'order_by'")
			})
		})
	})
})
