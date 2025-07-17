package handlers_test

import (
	"errors"
	"net/http"
	"strings"

	apierrors "code.cloudfoundry.org/korifi/api/errors"
	. "code.cloudfoundry.org/korifi/api/handlers"
	"code.cloudfoundry.org/korifi/api/handlers/fake"
	"code.cloudfoundry.org/korifi/api/payloads"
	"code.cloudfoundry.org/korifi/api/payloads/params"
	"code.cloudfoundry.org/korifi/api/repositories"
	"code.cloudfoundry.org/korifi/api/repositories/k8sklient/descriptors"
	"code.cloudfoundry.org/korifi/api/repositories/relationships"
	. "code.cloudfoundry.org/korifi/tests/matchers"
	"code.cloudfoundry.org/korifi/tools"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("ServicePlan", func() {
	var (
		servicePlanRepo     *fake.CFServicePlanRepository
		serviceOfferingRepo *fake.CFServiceOfferingRepository
		serviceBrokerRepo   *fake.CFServiceBrokerRepository
		spaceRepo           *fake.CFSpaceRepository
		orgRepo             *fake.CFOrgRepository
		requestValidator    *fake.RequestValidator
	)

	BeforeEach(func() {
		requestValidator = new(fake.RequestValidator)
		servicePlanRepo = new(fake.CFServicePlanRepository)
		serviceOfferingRepo = new(fake.CFServiceOfferingRepository)
		serviceBrokerRepo = new(fake.CFServiceBrokerRepository)
		spaceRepo = new(fake.CFSpaceRepository)
		orgRepo = new(fake.CFOrgRepository)

		apiHandler := NewServicePlan(
			*serverURL,
			requestValidator,
			servicePlanRepo,
			relationships.NewResourseRelationshipsRepo(
				serviceOfferingRepo,
				serviceBrokerRepo,
				servicePlanRepo,
				spaceRepo,
				orgRepo,
			),
		)
		routerBuilder.LoadRoutes(apiHandler)
	})

	Describe("GET /v3/service_plans", func() {
		BeforeEach(func() {
			servicePlanRepo.ListPlansReturns(repositories.ListResult[repositories.ServicePlanRecord]{
				Records: []repositories.ServicePlanRecord{{
					GUID:                "plan-guid",
					ServiceOfferingGUID: "service-offering-guid",
				}},
				PageInfo: descriptors.PageInfo{
					TotalResults: 1,
					TotalPages:   1,
					PageNumber:   1,
					PageSize:     1,
				},
			}, nil)

			serviceOfferingRepo.ListOfferingsReturns(repositories.ListResult[repositories.ServiceOfferingRecord]{
				Records: []repositories.ServiceOfferingRecord{{
					Name: "service-offering-name",
					GUID: "service-offering-guid",
				}},
				PageInfo: descriptors.PageInfo{
					TotalResults: 1,
					TotalPages:   1,
					PageNumber:   1,
					PageSize:     1,
				},
			}, nil)
		})

		JustBeforeEach(func() {
			req, err := http.NewRequestWithContext(ctx, "GET", "/v3/service_plans", nil)
			Expect(err).NotTo(HaveOccurred())

			routerBuilder.Build().ServeHTTP(rr, req)
		})

		It("lists the service plans", func() {
			Expect(servicePlanRepo.ListPlansCallCount()).To(Equal(1))
			_, actualAuthInfo, actualMessage := servicePlanRepo.ListPlansArgsForCall(0)
			Expect(actualAuthInfo).To(Equal(authInfo))
			Expect(actualMessage).To(Equal(repositories.ListServicePlanMessage{
				Pagination: repositories.Pagination{
					PerPage: 50,
					Page:    1,
				},
			}))

			Expect(rr).Should(HaveHTTPStatus(http.StatusOK))
			Expect(rr).To(HaveHTTPHeaderWithValue("Content-Type", "application/json"))
			Expect(rr).To(HaveHTTPBody(SatisfyAll(
				MatchJSONPath("$.pagination.total_results", BeEquivalentTo(1)),
				MatchJSONPath("$.resources[0].guid", "plan-guid"),
				MatchJSONPath("$.resources[0].links.self.href", "https://api.example.org/v3/service_plans/plan-guid"),
				MatchJSONPath("$.resources[0].links.service_offering.href", "https://api.example.org/v3/service_offerings/service-offering-guid"),
				Not(ContainSubstring("included")),
			)))
		})

		When("filtering query params are provided", func() {
			BeforeEach(func() {
				requestValidator.DecodeAndValidateURLValuesStub = decodeAndValidateURLValuesStub(&payloads.ServicePlanList{
					ServiceOfferingGUIDs: "a1,a2",
					BrokerGUIDs:          "b1-guid,b2-guid",
					BrokerNames:          "br1,br2",
					Names:                "n1,n2",
					ServiceOfferingNames: "so1,so2",
					Available:            tools.PtrTo(true),
					OrderBy:              "created_at",
					Pagination: payloads.Pagination{
						PerPage: "16",
						Page:    "32",
					},
				})
			})

			It("passes them to the repository", func() {
				Expect(servicePlanRepo.ListPlansCallCount()).To(Equal(1))
				_, _, message := servicePlanRepo.ListPlansArgsForCall(0)
				Expect(message).To(Equal(repositories.ListServicePlanMessage{
					Names:                []string{"n1", "n2"},
					ServiceOfferingGUIDs: []string{"a1", "a2"},
					ServiceOfferingNames: []string{"so1", "so2"},
					BrokerNames:          []string{"br1", "br2"},
					BrokerGUIDs:          []string{"b1-guid", "b2-guid"},
					Available:            tools.PtrTo(true),
					OrderBy:              "created_at",
					Pagination: repositories.Pagination{
						PerPage: 16,
						Page:    32,
					},
				}))
			})
		})

		When("params to include service_offering are provided", func() {
			BeforeEach(func() {
				requestValidator.DecodeAndValidateURLValuesStub = decodeAndValidateURLValuesStub(&payloads.ServicePlanList{
					IncludeResourceRules: []params.IncludeResourceRule{{
						RelationshipPath: []string{"service_offering"},
						Fields:           []string{},
					}},
				})
			})

			It("includes service offering in the response", func() {
				Expect(rr).Should(HaveHTTPStatus(http.StatusOK))
				Expect(rr).To(HaveHTTPBody(SatisfyAll(
					MatchJSONPath("$.included.service_offerings[0].guid", "service-offering-guid"),
					MatchJSONPath("$.included.service_offerings[0].name", "service-offering-name"),
				)))
			})
		})

		When("params to inlude fields[service_offering.service_broker]", func() {
			BeforeEach(func() {
				serviceBrokerRepo.ListServiceBrokersReturns(repositories.ListResult[repositories.ServiceBrokerRecord]{
					Records: []repositories.ServiceBrokerRecord{{
						Name: "service-broker-name",
						GUID: "service-broker-guid",
					}},
					PageInfo: descriptors.PageInfo{
						TotalResults: 1,
					},
				}, nil)

				serviceOfferingRepo.ListOfferingsReturns(repositories.ListResult[repositories.ServiceOfferingRecord]{
					Records: []repositories.ServiceOfferingRecord{{
						Name:              "service-offering-name",
						GUID:              "service-offering-guid",
						ServiceBrokerGUID: "service-broker-guid",
					}},
					PageInfo: descriptors.PageInfo{
						TotalResults: 1,
						TotalPages:   1,
						PageNumber:   1,
						PageSize:     1,
					},
				}, nil)

				requestValidator.DecodeAndValidateURLValuesStub = decodeAndValidateURLValuesStub(&payloads.ServicePlanList{
					IncludeResourceRules: []params.IncludeResourceRule{{
						RelationshipPath: []string{"service_offering", "service_broker"},
						Fields:           []string{"name", "guid"},
					}},
				})
			})

			It("includes broker fields in the response", func() {
				Expect(rr).Should(HaveHTTPStatus(http.StatusOK))
				Expect(rr).To(HaveHTTPBody(SatisfyAll(
					MatchJSONPath("$.included.service_brokers[0].guid", "service-broker-guid"),
					MatchJSONPath("$.included.service_brokers[0].name", "service-broker-name"),
				)))
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
				servicePlanRepo.ListPlansReturns(repositories.ListResult[repositories.ServicePlanRecord]{}, errors.New("list-err"))
			})

			It("returns an error", func() {
				expectUnknownError()
			})
		})
	})

	Describe("GET /v3/service_plans/{guid}", func() {
		BeforeEach(func() {
			servicePlanRepo.GetPlanReturns(repositories.ServicePlanRecord{
				GUID: "my-service-plan",
			}, nil)
		})

		JustBeforeEach(func() {
			req, err := http.NewRequestWithContext(ctx, "GET", "/v3/service_plans/my-service-plan", nil)
			Expect(err).NotTo(HaveOccurred())

			routerBuilder.Build().ServeHTTP(rr, req)
		})

		It("returns the plan", func() {
			Expect(servicePlanRepo.GetPlanCallCount()).To(Equal(1))
			_, actualAuthInfo, actualPlanID := servicePlanRepo.GetPlanArgsForCall(0)
			Expect(actualPlanID).To(Equal("my-service-plan"))
			Expect(actualAuthInfo).To(Equal(authInfo))

			Expect(rr).To(HaveHTTPStatus(http.StatusOK))
			Expect(rr).To(HaveHTTPHeaderWithValue("Content-Type", "application/json"))
		})

		When("getting the plan fails", func() {
			BeforeEach(func() {
				servicePlanRepo.GetPlanReturns(repositories.ServicePlanRecord{}, errors.New("unkown-err"))
			})

			It("returns an error", func() {
				expectUnknownError()
			})
		})
	})

	Describe("GET /v3/service_plans/{guid}/visibility", func() {
		BeforeEach(func() {
			servicePlanRepo.GetPlanReturns(repositories.ServicePlanRecord{
				Visibility: repositories.PlanVisibility{
					Type: korifiv1alpha1.AdminServicePlanVisibilityType,
				},
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
				Visibility: repositories.PlanVisibility{
					Type: korifiv1alpha1.PublicServicePlanVisibilityType,
				},
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

	Describe("PATCH /v3/service_plans/{guid}/visibility", func() {
		BeforeEach(func() {
			requestValidator.DecodeAndValidateJSONPayloadStub = decodeAndValidatePayloadStub(&payloads.ServicePlanVisibility{
				Type: korifiv1alpha1.PublicServicePlanVisibilityType,
			})

			servicePlanRepo.UpdatePlanVisibilityReturns(repositories.ServicePlanRecord{
				Visibility: repositories.PlanVisibility{
					Type: korifiv1alpha1.PublicServicePlanVisibilityType,
				},
			}, nil)
		})

		JustBeforeEach(func() {
			req, err := http.NewRequestWithContext(ctx, "PATCH", "/v3/service_plans/my-service-plan/visibility", strings.NewReader("the-payload"))
			Expect(err).NotTo(HaveOccurred())

			routerBuilder.Build().ServeHTTP(rr, req)
		})

		It("validates the payload", func() {
			Expect(requestValidator.DecodeAndValidateJSONPayloadCallCount()).To(Equal(1))
			actualReq, _ := requestValidator.DecodeAndValidateJSONPayloadArgsForCall(0)
			Expect(bodyString(actualReq)).To(Equal("the-payload"))
		})

		It("updates the plan visibility", func() {
			Expect(servicePlanRepo.UpdatePlanVisibilityCallCount()).To(Equal(1))
			_, actualAuthInfo, actualMessage := servicePlanRepo.UpdatePlanVisibilityArgsForCall(0)
			Expect(actualAuthInfo).To(Equal(authInfo))
			Expect(actualMessage).To(Equal(repositories.UpdateServicePlanVisibilityMessage{
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
				servicePlanRepo.UpdatePlanVisibilityReturns(repositories.ServicePlanRecord{}, errors.New("visibility-err"))
			})

			It("returns an error", func() {
				expectUnknownError()
			})
		})
	})

	Describe("DELETE /v3/service_plans/{guid}/visibility/{org-guid}", func() {
		JustBeforeEach(func() {
			req, err := http.NewRequestWithContext(ctx, "DELETE", "/v3/service_plans/my-service-plan/visibility/org-guid", nil)
			Expect(err).NotTo(HaveOccurred())

			routerBuilder.Build().ServeHTTP(rr, req)
		})

		It("deletes the service plan visibility", func() {
			Expect(servicePlanRepo.DeletePlanVisibilityCallCount()).To(Equal(1))
			_, actualAuthInfo, actualMessage := servicePlanRepo.DeletePlanVisibilityArgsForCall(0)
			Expect(actualAuthInfo).To(Equal(authInfo))
			Expect(actualMessage.PlanGUID).To(Equal("my-service-plan"))
			Expect(actualMessage.OrgGUID).To(Equal("org-guid"))
			Expect(rr).To(HaveHTTPStatus(http.StatusNoContent))
		})

		When("deleting the visibility fails with an error", func() {
			BeforeEach(func() {
				servicePlanRepo.DeletePlanVisibilityReturns(errors.New("visibility-err"))
			})

			It("returns an error", func() {
				expectUnknownError()
			})
		})
	})

	Describe("DELETE /v3/service_plans/:guid", func() {
		BeforeEach(func() {
			servicePlanRepo.GetPlanReturns(repositories.ServicePlanRecord{
				GUID:                "plan-guid",
				ServiceOfferingGUID: "service-offering-guid",
			}, nil)
		})

		JustBeforeEach(func() {
			req, err := http.NewRequestWithContext(ctx, "DELETE", "/v3/service_plans/plan-guid", nil)
			Expect(err).NotTo(HaveOccurred())

			routerBuilder.Build().ServeHTTP(rr, req)
		})

		It("deletes the service plan", func() {
			Expect(servicePlanRepo.DeletePlanCallCount()).To(Equal(1))
			_, actualAuthInfo, actualPlanGUID := servicePlanRepo.DeletePlanArgsForCall(0)
			Expect(actualAuthInfo).To(Equal(authInfo))
			Expect(actualPlanGUID).To(Equal("plan-guid"))

			Expect(rr).Should(HaveHTTPStatus(http.StatusNoContent))
		})

		When("deleting the service plan fails with not found", func() {
			BeforeEach(func() {
				servicePlanRepo.DeletePlanReturns(
					apierrors.NewNotFoundError(errors.New("not found"), repositories.ServicePlanResourceType),
				)
			})

			It("returns 404 Not Found", func() {
				expectNotFoundError("Service Plan")
			})
		})

		When("deleting the service plan fails", func() {
			BeforeEach(func() {
				servicePlanRepo.DeletePlanReturns(errors.New("boom"))
			})

			It("returns 500 Internal Server Error", func() {
				expectUnknownError()
			})
		})
	})
})
