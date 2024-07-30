package handlers_test

import (
	"errors"
	"net/http"

	. "code.cloudfoundry.org/korifi/api/handlers"
	"code.cloudfoundry.org/korifi/api/handlers/fake"
	"code.cloudfoundry.org/korifi/api/repositories"
	"code.cloudfoundry.org/korifi/model"
	. "code.cloudfoundry.org/korifi/tests/matchers"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("ServicePlan", func() {
	var servicePlanRepo *fake.CFServicePlanRepository

	BeforeEach(func() {
		servicePlanRepo = new(fake.CFServicePlanRepository)

		apiHandler := NewServicePlan(
			*serverURL,
			servicePlanRepo,
		)
		routerBuilder.LoadRoutes(apiHandler)
	})

	Describe("GET /v3/service_plans", func() {
		BeforeEach(func() {
			servicePlanRepo.ListPlansReturns([]repositories.ServicePlanRecord{{
				CFResource: model.CFResource{
					GUID: "plan-guid",
				},
				Relationships: repositories.ServicePlanRelationships{
					ServiceOffering: model.ToOneRelationship{
						Data: model.Relationship{
							GUID: "service-offering-guid",
						},
					},
				},
			}}, nil)
		})

		JustBeforeEach(func() {
			req, err := http.NewRequestWithContext(ctx, "GET", "/v3/service_plans", nil)
			Expect(err).NotTo(HaveOccurred())

			routerBuilder.Build().ServeHTTP(rr, req)
		})

		It("lists the service plans", func() {
			Expect(servicePlanRepo.ListPlansCallCount()).To(Equal(1))
			_, actualAuthInfo := servicePlanRepo.ListPlansArgsForCall(0)
			Expect(actualAuthInfo).To(Equal(authInfo))

			Expect(rr).Should(HaveHTTPStatus(http.StatusOK))
			Expect(rr).To(HaveHTTPHeaderWithValue("Content-Type", "application/json"))
			Expect(rr).To(HaveHTTPBody(SatisfyAll(
				MatchJSONPath("$.pagination.total_results", BeEquivalentTo(1)),
				MatchJSONPath("$.pagination.first.href", "https://api.example.org/v3/service_plans"),
				MatchJSONPath("$.resources[0].guid", "plan-guid"),
				MatchJSONPath("$.resources[0].links.self.href", "https://api.example.org/v3/service_plans/plan-guid"),
				MatchJSONPath("$.resources[0].links.service_offering.href", "https://api.example.org/v3/service_offerings/service-offering-guid"),
			)))
		})

		When("listing the plans fails", func() {
			BeforeEach(func() {
				servicePlanRepo.ListPlansReturns(nil, errors.New("list-err"))
			})

			It("returns an error", func() {
				expectUnknownError()
			})
		})
	})
})
