package handlers_test

import (
	"errors"
	"net/http"
	"net/http/httptest"

	. "code.cloudfoundry.org/korifi/api/handlers"
	"code.cloudfoundry.org/korifi/api/handlers/fake"
	"code.cloudfoundry.org/korifi/api/payloads"
	"code.cloudfoundry.org/korifi/api/repositories"
	. "code.cloudfoundry.org/korifi/tests/matchers"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Buildpack", func() {
	var (
		buildpackRepo    *fake.BuildpackRepository
		req              *http.Request
		requestValidator *fake.RequestValidator
	)

	BeforeEach(func() {
		buildpackRepo = new(fake.BuildpackRepository)

		requestValidator = new(fake.RequestValidator)
		apiHandler := NewBuildpack(*serverURL, buildpackRepo, requestValidator)
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

		It("validates the request", func() {
			Expect(requestValidator.DecodeAndValidateURLValuesCallCount()).To(Equal(1))
			actualReq, _ := requestValidator.DecodeAndValidateURLValuesArgsForCall(0)
			Expect(actualReq.URL).To(Equal(req.URL))
		})

		It("returns the buildpacks for the default builder", func() {
			Expect(buildpackRepo.ListBuildpacksCallCount()).To(Equal(1))
			_, actualAuthInfo := buildpackRepo.ListBuildpacksArgsForCall(0)
			Expect(actualAuthInfo).To(Equal(authInfo))

			Expect(rr).To(HaveHTTPStatus(http.StatusOK))
			Expect(rr).To(HaveHTTPHeaderWithValue("Content-Type", "application/json"))
			Expect(rr).To(HaveHTTPBody(SatisfyAll(
				MatchJSONPath("$.pagination.total_results", BeEquivalentTo(1)),
				MatchJSONPath("$.pagination.first.href", "https://api.example.org/v3/buildpacks"),
				MatchJSONPath("$.resources", HaveLen(1)),
				MatchJSONPath("$.resources[0].filename", "paketo-foopacks/bar@1.0.0"),
			)))
		})

		Describe("Order results", func() {
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

			DescribeTable("ordering results", func(orderBy string, expectedOrder ...any) {
				req = createHttpRequest("GET", "/v3/buildpacks?order_by=not-used", nil)
				requestValidator.DecodeAndValidateURLValuesStub = decodeAndValidateURLValuesStub(&payloads.BuildpackList{
					OrderBy: orderBy,
				})
				rr = httptest.NewRecorder()
				routerBuilder.Build().ServeHTTP(rr, req)
				Expect(rr).To(HaveHTTPBody(MatchJSONPath("$.resources[*].position", expectedOrder)))
			},
				Entry("created_at ASC", "created_at", 1.0, 3.0, 2.0),
				Entry("created_at DESC", "-created_at", 2.0, 3.0, 1.0),
				Entry("updated_at ASC", "updated_at", 2.0, 1.0, 3.0),
				Entry("updated_at DESC", "-updated_at", 3.0, 1.0, 2.0),
				Entry("position ASC", "position", 1.0, 2.0, 3.0),
				Entry("position DESC", "-position", 3.0, 2.0, 1.0),
			)
		})

		When("request is invalid", func() {
			BeforeEach(func() {
				requestValidator.DecodeAndValidateURLValuesReturns(errors.New("foo"))
			})

			It("returns an Unknown error", func() {
				expectUnknownError()
			})
		})
	})
})
