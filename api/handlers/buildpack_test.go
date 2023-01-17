package handlers_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"

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

		Describe("Order results", func() {
			type res struct {
				Position int `json:"position"`
			}
			type resList struct {
				Resources []res `json:"resources"`
			}

			BeforeEach(func() {
				buildpackRepo.ListBuildpacksReturns([]repositories.BuildpackRecord{
					{
						CreatedAt: "2023-01-14T14:58:32Z",
						UpdatedAt: "2023-01-19T14:58:32Z",
						Position:  1,
					},
					{
						CreatedAt: "2023-01-17T14:57:32Z",
						UpdatedAt: "2023-01-18:57:32Z",
						Position:  2,
					},
					{
						CreatedAt: "2023-01-16T14:57:32Z",
						UpdatedAt: "2023-01-20:57:32Z",
						Position:  3,
					},
				}, nil)
			})

			DescribeTable("ordering results", func(orderBy string, expectedOrder ...int) {
				req = createHttpRequest("GET", "/v3/buildpacks?order_by="+orderBy, nil)
				rr = httptest.NewRecorder()
				routerBuilder.Build().ServeHTTP(rr, req)
				var respList resList
				err := json.Unmarshal(rr.Body.Bytes(), &respList)
				Expect(err).NotTo(HaveOccurred())
				expectedList := make([]res, len(expectedOrder))
				for i := range expectedOrder {
					expectedList[i] = res{Position: expectedOrder[i]}
				}
				Expect(respList.Resources).To(Equal(expectedList))
			},
				Entry("created_at ASC", "created_at", 1, 3, 2),
				Entry("created_at DESC", "-created_at", 2, 3, 1),
				Entry("updated_at ASC", "updated_at", 2, 1, 3),
				Entry("updated_at DESC", "-updated_at", 3, 1, 2),
				Entry("position ASC", "position", 1, 2, 3),
				Entry("position DESC", "-position", 3, 2, 1),
			)

			When("order_by is not a valid field", func() {
				BeforeEach(func() {
					req = createHttpRequest("GET", "/v3/buildpacks?order_by=not_valid", nil)
				})

				It("returns an Unknown key error", func() {
					expectUnknownKeyError("The query parameter is invalid: Order by can only be: 'created_at', 'updated_at', 'position'")
				})
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
