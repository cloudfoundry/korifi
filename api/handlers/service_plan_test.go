package handlers_test

import (
	"errors"
	"net/http"
	"strings"

	. "code.cloudfoundry.org/korifi/api/handlers"
	"code.cloudfoundry.org/korifi/api/handlers/fake"
	"code.cloudfoundry.org/korifi/api/payloads"
	"code.cloudfoundry.org/korifi/api/repositories"
	"code.cloudfoundry.org/korifi/model"
	. "code.cloudfoundry.org/korifi/tests/matchers"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("ServicePlan", func() {
	var (
		servicePlanRepo  *fake.CFServicePlanRepository
		requestValidator *fake.RequestValidator
	)

	BeforeEach(func() {
		requestValidator = new(fake.RequestValidator)
		servicePlanRepo = new(fake.CFServicePlanRepository)

		apiHandler := NewServicePlan(
			*serverURL,
			requestValidator,
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
			_, actualAuthInfo, _ := servicePlanRepo.ListPlansArgsForCall(0)
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

		When("filtering query params are provided", func() {
			BeforeEach(func() {
				requestValidator.DecodeAndValidateURLValuesStub = decodeAndValidateURLValuesStub(&payloads.ServicePlanList{
					ServiceOfferingGUIDs: "a1,a2",
				})
			})

			It("passes them to the repository", func() {
				Expect(servicePlanRepo.ListPlansCallCount()).To(Equal(1))
				_, _, message := servicePlanRepo.ListPlansArgsForCall(0)
				Expect(message.ServiceOfferingGUIDs).To(ConsistOf("a1", "a2"))
			})
		})

		When("the request is invalid", func() {
			BeforeEach(func() {
				requestValidator.DecodeAndValidateURLValuesReturns(errors.New("invalid-request"))
			})

			It("returns an error", func() {
				expectUnknownError()
			})
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

	Describe("GET /v3/service_plans/{guid}/visibility", func() {
		BeforeEach(func() {
			servicePlanRepo.GetPlanReturns(repositories.ServicePlanRecord{
				VisibilityType: korifiv1alpha1.AdminServicePlanVisibilityType,
			}, nil)
		})

		JustBeforeEach(func() {
			req, err := http.NewRequestWithContext(ctx, "GET", "/v3/service_plans/my-service-plan/visibility", nil)
			Expect(err).NotTo(HaveOccurred())

			routerBuilder.Build().ServeHTTP(rr, req)
		})

		It("returns the plan visibility", func() {
			Expect(servicePlanRepo.GetPlanCallCount()).To(Equal(1))
			_, actualAuthInfo, actualPlanID := servicePlanRepo.GetPlanArgsForCall(0)
			Expect(actualPlanID).To(Equal("my-service-plan"))
			Expect(actualAuthInfo).To(Equal(authInfo))

			Expect(rr).To(HaveHTTPStatus(http.StatusOK))
			Expect(rr).To(HaveHTTPHeaderWithValue("Content-Type", "application/json"))

			Expect(rr).To(HaveHTTPBody(SatisfyAll(
				MatchJSONPath("$.type", korifiv1alpha1.AdminServicePlanVisibilityType),
			)))
		})

		When("getting the visibility fails", func() {
			BeforeEach(func() {
				servicePlanRepo.GetPlanReturns(repositories.ServicePlanRecord{}, errors.New("visibility-err"))
			})

			It("returns an error", func() {
				expectUnknownError()
			})
		})
	})

	Describe("POST /v3/service_plans/{guid}/visibility", func() {
		BeforeEach(func() {
			requestValidator.DecodeAndValidateJSONPayloadStub = decodeAndValidatePayloadStub(&payloads.ServicePlanVisibility{
				Type: korifiv1alpha1.PublicServicePlanVisibilityType,
			})

			servicePlanRepo.ApplyPlanVisibilityReturns(repositories.ServicePlanRecord{
				VisibilityType: korifiv1alpha1.PublicServicePlanVisibilityType,
			}, nil)
		})

		JustBeforeEach(func() {
			req, err := http.NewRequestWithContext(ctx, "POST", "/v3/service_plans/my-service-plan/visibility", strings.NewReader("the-payload"))
			Expect(err).NotTo(HaveOccurred())

			routerBuilder.Build().ServeHTTP(rr, req)
		})

		It("validates the payload", func() {
			Expect(requestValidator.DecodeAndValidateJSONPayloadCallCount()).To(Equal(1))
			actualReq, _ := requestValidator.DecodeAndValidateJSONPayloadArgsForCall(0)
			Expect(bodyString(actualReq)).To(Equal("the-payload"))
		})

		It("updates the plan visibility", func() {
			Expect(servicePlanRepo.ApplyPlanVisibilityCallCount()).To(Equal(1))
			_, actualAuthInfo, actualMessage := servicePlanRepo.ApplyPlanVisibilityArgsForCall(0)
			Expect(actualAuthInfo).To(Equal(authInfo))
			Expect(actualMessage).To(Equal(repositories.ApplyServicePlanVisibilityMessage{
				PlanGUID: "my-service-plan",
				Type:     korifiv1alpha1.PublicServicePlanVisibilityType,
			}))

			Expect(rr).To(HaveHTTPStatus(http.StatusOK))
			Expect(rr).To(HaveHTTPHeaderWithValue("Content-Type", "application/json"))

			Expect(rr).To(HaveHTTPBody(SatisfyAll(
				MatchJSONPath("$.type", korifiv1alpha1.PublicServicePlanVisibilityType),
			)))
		})

		When("the payload is invalid", func() {
			BeforeEach(func() {
				requestValidator.DecodeAndValidateJSONPayloadReturns(errors.New("invalid-payload"))
			})

			It("returns an error", func() {
				expectUnknownError()
			})
		})

		When("updating the visibility fails", func() {
			BeforeEach(func() {
				servicePlanRepo.ApplyPlanVisibilityReturns(repositories.ServicePlanRecord{}, errors.New("visibility-err"))
			})

			It("returns an error", func() {
				expectUnknownError()
			})
		})
	})
})
