package handlers_test

import (
	"errors"
	"net/http"
	"time"

	. "code.cloudfoundry.org/korifi/api/handlers"
	"code.cloudfoundry.org/korifi/api/handlers/fake"
	"code.cloudfoundry.org/korifi/api/payloads"
	"code.cloudfoundry.org/korifi/api/repositories"
	"code.cloudfoundry.org/korifi/api/repositories/k8sklient/descriptors"
	. "code.cloudfoundry.org/korifi/tests/matchers"
	"code.cloudfoundry.org/korifi/tools"

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
			buildpackRepo.ListBuildpacksReturns(
				repositories.ListResult[repositories.BuildpackRecord]{
					Records: []repositories.BuildpackRecord{{
						Name:      "paketo-foopacks/bar",
						Position:  1,
						Stack:     "waffle-house",
						Version:   "1.0.0",
						CreatedAt: time.UnixMilli(1000),
						UpdatedAt: tools.PtrTo(time.UnixMilli(2000)),
					}},
					PageInfo: descriptors.PageInfo{
						TotalResults: 1,
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
			_, actualAuthInfo, actualMessage := buildpackRepo.ListBuildpacksArgsForCall(0)
			Expect(actualAuthInfo).To(Equal(authInfo))
			Expect(actualMessage).To(Equal(repositories.ListBuildpacksMessage{
				Pagination: repositories.Pagination{
					PerPage: 50,
					Page:    1,
				},
			}))

			Expect(rr).To(HaveHTTPStatus(http.StatusOK))
			Expect(rr).To(HaveHTTPHeaderWithValue("Content-Type", "application/json"))
			Expect(rr).To(HaveHTTPBody(SatisfyAll(
				MatchJSONPath("$.pagination.total_results", BeEquivalentTo(1)),
				MatchJSONPath("$.resources", HaveLen(1)),
				MatchJSONPath("$.resources[0].filename", "paketo-foopacks/bar@1.0.0"),
			)))
		})

		When("filtering query params are provided", func() {
			BeforeEach(func() {
				requestValidator.DecodeAndValidateURLValuesStub = decodeAndValidateURLValuesStub(&payloads.BuildpackList{
					OrderBy:    "created_at",
					Pagination: payloads.Pagination{PerPage: "16", Page: "32"},
				})
			})

			It("passes them to the repository", func() {
				Expect(buildpackRepo.ListBuildpacksCallCount()).To(Equal(1))
				_, _, message := buildpackRepo.ListBuildpacksArgsForCall(0)
				Expect(message).To(Equal(repositories.ListBuildpacksMessage{
					OrderBy:    "created_at",
					Pagination: repositories.Pagination{PerPage: 16, Page: 32},
				}))
			})
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
