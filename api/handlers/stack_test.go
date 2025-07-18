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

var _ = Describe("Stack", func() {
	var (
		stackRepo        *fake.StackRepository
		requestValidator *fake.RequestValidator
		req              *http.Request
	)

	BeforeEach(func() {
		stackRepo = new(fake.StackRepository)
		requestValidator = new(fake.RequestValidator)

		apiHandler := NewStack(*serverURL, stackRepo, requestValidator)
		routerBuilder.LoadRoutes(apiHandler)
	})

	JustBeforeEach(func() {
		routerBuilder.Build().ServeHTTP(rr, req)
	})

	Describe("the GET /v3/stacks endpoint", func() {
		BeforeEach(func() {
			stackRepo.ListStacksReturns(repositories.ListResult[repositories.StackRecord]{
				Records: []repositories.StackRecord{{
					Name:        "io.buildpacks.stacks.jammy",
					Description: "Jammy Stack",
					CreatedAt:   time.UnixMilli(1000),
					UpdatedAt:   tools.PtrTo(time.UnixMilli(2000)),
				}},
				PageInfo: descriptors.PageInfo{
					TotalResults: 1,
					TotalPages:   1,
					PageNumber:   1,
					PageSize:     1,
				},
			}, nil)

			var err error
			req, err = http.NewRequestWithContext(ctx, "GET", "/v3/stacks", nil)
			Expect(err).NotTo(HaveOccurred())
		})

		It("returns the stacks for the default builder", func() {
			Expect(stackRepo.ListStacksCallCount()).To(Equal(1))
			_, actualAuthInfo, actualMessage := stackRepo.ListStacksArgsForCall(0)
			Expect(actualAuthInfo).To(Equal(authInfo))
			Expect(actualMessage).To(Equal(repositories.ListStacksMessage{
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
				MatchJSONPath("$.resources[0].name", "io.buildpacks.stacks.jammy"),
			)))
		})

		When("paging is requested", func() {
			BeforeEach(func() {
				requestValidator.DecodeAndValidateURLValuesStub = decodeAndValidateURLValuesStub(&payloads.StackList{
					Pagination: payloads.Pagination{
						PerPage: "20",
						Page:    "1",
					},
				})
			})

			It("passes them to the repository", func() {
				Expect(stackRepo.ListStacksCallCount()).To(Equal(1))
				_, _, message := stackRepo.ListStacksArgsForCall(0)
				Expect(message).To(Equal(repositories.ListStacksMessage{
					Pagination: repositories.Pagination{
						PerPage: 20,
						Page:    1,
					},
				}))
			})
		})

		When("there is some other error fetching the stacks", func() {
			BeforeEach(func() {
				stackRepo.ListStacksReturns(repositories.ListResult[repositories.StackRecord]{}, errors.New("unknown!"))
			})

			It("returns an error", func() {
				expectUnknownError()
			})
		})
	})
})
