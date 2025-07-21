package handlers_test

import (
	"errors"
	"net/http"

	"code.cloudfoundry.org/korifi/api/handlers"
	"code.cloudfoundry.org/korifi/api/handlers/fake"
	"code.cloudfoundry.org/korifi/api/payloads"
	"code.cloudfoundry.org/korifi/api/repositories"
	"code.cloudfoundry.org/korifi/api/repositories/k8sklient/descriptors"
	. "code.cloudfoundry.org/korifi/tests/matchers"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("User", func() {
	var (
		userRepo         *fake.UserRepository
		requestValidator *fake.RequestValidator
		req              *http.Request
	)

	BeforeEach(func() {
		userRepo = new(fake.UserRepository)
		requestValidator = new(fake.RequestValidator)

		apiHandler := handlers.NewUser(*serverURL, userRepo, requestValidator)
		routerBuilder.LoadRoutes(apiHandler)
	})

	JustBeforeEach(func() {
		routerBuilder.Build().ServeHTTP(rr, req)
	})

	Describe("GET /v3/users", func() {
		BeforeEach(func() {
			userRepo.ListUsersReturns(repositories.ListResult[repositories.UserRecord]{
				Records: []repositories.UserRecord{{
					GUID: "user-1",
					Name: "user-1",
				}},
				PageInfo: descriptors.PageInfo{
					TotalResults: 1,
					TotalPages:   1,
					PageNumber:   1,
					PageSize:     1,
				},
			}, nil)

			var err error
			req, err = http.NewRequestWithContext(ctx, "GET", "/v3/users?foo=bar", nil)
			Expect(err).NotTo(HaveOccurred())
		})

		It("returns the users", func() {
			Expect(requestValidator.DecodeAndValidateURLValuesCallCount()).To(Equal(1))
			actualReq, _ := requestValidator.DecodeAndValidateURLValuesArgsForCall(0)
			Expect(actualReq.URL.String()).To(HaveSuffix("foo=bar"))

			Expect(userRepo.ListUsersCallCount()).To(Equal(1))
			_, actualAuthInfo, actualMessage := userRepo.ListUsersArgsForCall(0)
			Expect(actualAuthInfo).To(Equal(authInfo))
			Expect(actualMessage).To(Equal(repositories.ListUsersMessage{
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
				MatchJSONPath("$.resources[0].guid", "user-1"),
			)))
		})

		When("paging is requested", func() {
			BeforeEach(func() {
				requestValidator.DecodeAndValidateURLValuesStub = decodeAndValidateURLValuesStub(&payloads.UserList{
					Pagination: payloads.Pagination{
						PerPage: "20",
						Page:    "1",
					},
				})
			})

			It("passes them to the repository", func() {
				Expect(userRepo.ListUsersCallCount()).To(Equal(1))
				_, _, message := userRepo.ListUsersArgsForCall(0)
				Expect(message).To(Equal(repositories.ListUsersMessage{
					Pagination: repositories.Pagination{
						PerPage: 20,
						Page:    1,
					},
				}))
			})
		})

		When("listing users fails", func() {
			BeforeEach(func() {
				userRepo.ListUsersReturns(repositories.ListResult[repositories.UserRecord]{}, errors.New("unknown!"))
			})

			It("returns an error", func() {
				expectUnknownError()
			})
		})
	})
})
