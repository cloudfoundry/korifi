package handlers_test

import (
	"errors"
	"net/http"
	"time"

	. "code.cloudfoundry.org/korifi/api/handlers"
	"code.cloudfoundry.org/korifi/api/handlers/fake"
	"code.cloudfoundry.org/korifi/api/repositories"
	. "code.cloudfoundry.org/korifi/tests/matchers"
	"code.cloudfoundry.org/korifi/tools"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Stack", func() {
	var (
		stackRepo *fake.StackRepository
		req       *http.Request
	)

	BeforeEach(func() {
		stackRepo = new(fake.StackRepository)

		apiHandler := NewStack(*serverURL, stackRepo)
		routerBuilder.LoadRoutes(apiHandler)
	})

	JustBeforeEach(func() {
		routerBuilder.Build().ServeHTTP(rr, req)
	})

	Describe("the GET /v3/stacks endpoint", func() {
		BeforeEach(func() {
			stackRepo.ListStacksReturns([]repositories.StackRecord{
				{
					Name:        "io.buildpacks.stacks.jammy",
					Description: "Jammy Stack",
					CreatedAt:   time.UnixMilli(1000),
					UpdatedAt:   tools.PtrTo(time.UnixMilli(2000)),
				},
			}, nil)

			var err error
			req, err = http.NewRequestWithContext(ctx, "GET", "/v3/stacks", nil)
			Expect(err).NotTo(HaveOccurred())
		})

		It("returns the stacks for the default builder", func() {
			Expect(stackRepo.ListStacksCallCount()).To(Equal(1))
			_, actualAuthInfo := stackRepo.ListStacksArgsForCall(0)
			Expect(actualAuthInfo).To(Equal(authInfo))

			Expect(rr).To(HaveHTTPStatus(http.StatusOK))
			Expect(rr).To(HaveHTTPHeaderWithValue("Content-Type", "application/json"))
			Expect(rr).To(HaveHTTPBody(SatisfyAll(
				MatchJSONPath("$.pagination.total_results", BeEquivalentTo(1)),
				MatchJSONPath("$.pagination.first.href", "https://api.example.org/v3/stacks"),
				MatchJSONPath("$.resources", HaveLen(1)),
				MatchJSONPath("$.resources[0].name", "io.buildpacks.stacks.jammy"),
			)))
		})
		When("there is some other error fetching the stacks", func() {
			BeforeEach(func() {
				stackRepo.ListStacksReturns([]repositories.StackRecord{}, errors.New("unknown!"))
			})

			It("returns an error", func() {
				expectUnknownError()
			})
		})
	})
})
