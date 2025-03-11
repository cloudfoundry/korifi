package handlers_test

import (
	"errors"
	"net/http"

	apierrors "code.cloudfoundry.org/korifi/api/errors"
	. "code.cloudfoundry.org/korifi/api/handlers"
	"code.cloudfoundry.org/korifi/api/handlers/fake"
	"code.cloudfoundry.org/korifi/api/payloads"
	"code.cloudfoundry.org/korifi/api/payloads/params"
	"code.cloudfoundry.org/korifi/api/repositories"
	"code.cloudfoundry.org/korifi/api/repositories/relationships"
	. "code.cloudfoundry.org/korifi/tests/matchers"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("ServiceOffering", func() {
	var (
		requestValidator    *fake.RequestValidator
		serviceOfferingRepo *fake.CFServiceOfferingRepository
		serviceBrokerRepo   *fake.CFServiceBrokerRepository
		servicePlanRepo     *fake.CFServicePlanRepository
	)

	BeforeEach(func() {
		requestValidator = new(fake.RequestValidator)
		serviceOfferingRepo = new(fake.CFServiceOfferingRepository)
		serviceBrokerRepo = new(fake.CFServiceBrokerRepository)
		servicePlanRepo = new(fake.CFServicePlanRepository)

		apiHandler := NewServiceOffering(
			*serverURL,
			requestValidator,
			serviceOfferingRepo,
			serviceBrokerRepo,
			relationships.NewResourseRelationshipsRepo(
				serviceOfferingRepo,
				serviceBrokerRepo,
				servicePlanRepo,
			),
		)
		routerBuilder.LoadRoutes(apiHandler)
	})

	Describe("GET /v3/service_offering/:guid", func() {
		BeforeEach(func() {
			serviceOfferingRepo.GetServiceOfferingReturns(repositories.ServiceOfferingRecord{
				GUID:              "offering-guid",
				ServiceBrokerGUID: "broker-guid",
			}, nil)
		})

		JustBeforeEach(func() {
			req, err := http.NewRequestWithContext(ctx, "GET", "/v3/service_offerings/offering-guid", nil)
			Expect(err).NotTo(HaveOccurred())

			routerBuilder.Build().ServeHTTP(rr, req)
		})

		It("returns the service offering", func() {
			Expect(serviceOfferingRepo.GetServiceOfferingCallCount()).To(Equal(1))
			_, actualAuthInfo, actualOfferingGUID := serviceOfferingRepo.GetServiceOfferingArgsForCall(0)
			Expect(actualAuthInfo).To(Equal(authInfo))
			Expect(actualOfferingGUID).To(Equal("offering-guid"))

			Expect(rr).Should(HaveHTTPStatus(http.StatusOK))
			Expect(rr).To(HaveHTTPHeaderWithValue("Content-Type", "application/json"))
			Expect(rr).To(HaveHTTPBody(SatisfyAll(
				MatchJSONPath("$.guid", "offering-guid"),
			)))
		})

		When("params to include fields[service_broker]", func() {
			BeforeEach(func() {
				serviceBrokerRepo.ListServiceBrokersReturns([]repositories.ServiceBrokerRecord{{
					Name: "broker-name",
					GUID: "broker-guid",
				}}, nil)

				requestValidator.DecodeAndValidateURLValuesStub = decodeAndValidateURLValuesStub(&payloads.ServiceOfferingGet{
					IncludeResourceRules: []params.IncludeResourceRule{{
						RelationshipPath: []string{"service_broker"},
						Fields:           []string{"name", "guid"},
					}},
				})
			})

			It("includes service offering in the response", func() {
				Expect(rr).Should(HaveHTTPStatus(http.StatusOK))
				Expect(rr).To(HaveHTTPBody(SatisfyAll(
					MatchJSONPath("$.included.service_brokers[0].name", "broker-name"),
					MatchJSONPath("$.included.service_brokers[0].guid", "broker-guid"),
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

		When("getting the offering fails", func() {
			BeforeEach(func() {
				serviceOfferingRepo.GetServiceOfferingReturns(repositories.ServiceOfferingRecord{}, errors.New("get-err"))
			})

			It("returns an error", func() {
				expectUnknownError()
			})
		})
	})

	Describe("GET /v3/service_offerings", func() {
		BeforeEach(func() {
			serviceOfferingRepo.ListOfferingsReturns([]repositories.ServiceOfferingRecord{{
				GUID:              "offering-guid",
				ServiceBrokerGUID: "broker-guid",
			}}, nil)
		})

		JustBeforeEach(func() {
			req, err := http.NewRequestWithContext(ctx, "GET", "/v3/service_offerings", nil)
			Expect(err).NotTo(HaveOccurred())

			routerBuilder.Build().ServeHTTP(rr, req)
		})

		It("lists the service offerings", func() {
			Expect(serviceOfferingRepo.ListOfferingsCallCount()).To(Equal(1))
			_, actualAuthInfo, _ := serviceOfferingRepo.ListOfferingsArgsForCall(0)
			Expect(actualAuthInfo).To(Equal(authInfo))

			Expect(rr).Should(HaveHTTPStatus(http.StatusOK))
			Expect(rr).To(HaveHTTPHeaderWithValue("Content-Type", "application/json"))
			Expect(rr).To(HaveHTTPBody(SatisfyAll(
				MatchJSONPath("$.pagination.total_results", BeEquivalentTo(1)),
				MatchJSONPath("$.pagination.first.href", "https://api.example.org/v3/service_offerings"),
				MatchJSONPath("$.resources[0].guid", "offering-guid"),
				MatchJSONPath("$.resources[0].links.self.href", "https://api.example.org/v3/service_offerings/offering-guid"),
				MatchJSONPath("$.resources[0].links.service_plans.href", "https://api.example.org/v3/service_plans?service_offering_guids=offering-guid"),
				MatchJSONPath("$.resources[0].links.service_broker.href", "https://api.example.org/v3/service_brokers/broker-guid"),
			)))
		})

		When("filtering query params are provided", func() {
			BeforeEach(func() {
				requestValidator.DecodeAndValidateURLValuesStub = decodeAndValidateURLValuesStub(&payloads.ServiceOfferingList{
					Names: "a1,a2",
				})
			})

			It("passes them to the repository", func() {
				Expect(serviceOfferingRepo.ListOfferingsCallCount()).To(Equal(1))
				_, _, message := serviceOfferingRepo.ListOfferingsArgsForCall(0)
				Expect(message.Names).To(ConsistOf("a1", "a2"))
			})
		})

		Describe("include broker fields", func() {
			BeforeEach(func() {
				serviceBrokerRepo.ListServiceBrokersReturns([]repositories.ServiceBrokerRecord{{
					Name: "broker-name",
					GUID: "broker-guid",
				}}, nil)

				requestValidator.DecodeAndValidateURLValuesStub = decodeAndValidateURLValuesStub(&payloads.ServiceOfferingList{
					IncludeResourceRules: []params.IncludeResourceRule{{
						RelationshipPath: []string{"service_broker"},
						Fields:           []string{"name", "guid"},
					}},
				})
			})

			It("lists the brokers", func() {
				Expect(serviceBrokerRepo.ListServiceBrokersCallCount()).To(Equal(1))
				_, _, actualListMessage := serviceBrokerRepo.ListServiceBrokersArgsForCall(0)
				Expect(actualListMessage).To(Equal(repositories.ListServiceBrokerMessage{
					GUIDs: []string{"broker-guid"},
				}))
			})

			When("listing brokers fails", func() {
				BeforeEach(func() {
					serviceBrokerRepo.ListServiceBrokersReturns([]repositories.ServiceBrokerRecord{}, errors.New("list-broker-err"))
				})

				It("returns an error", func() {
					expectUnknownError()
				})
			})

			Describe("broker name", func() {
				BeforeEach(func() {
					requestValidator.DecodeAndValidateURLValuesStub = decodeAndValidateURLValuesStub(&payloads.ServiceOfferingList{
						IncludeResourceRules: []params.IncludeResourceRule{{
							RelationshipPath: []string{"service_broker"},
							Fields:           []string{"name"},
						}},
					})
				})

				It("includes broker fields in the response", func() {
					Expect(rr).Should(HaveHTTPStatus(http.StatusOK))
					Expect(rr).To(HaveHTTPBody(SatisfyAll(
						MatchJSONPath("$.included.service_brokers[0].name", "broker-name"),
					)))
				})
			})

			Describe("broker guid", func() {
				BeforeEach(func() {
					requestValidator.DecodeAndValidateURLValuesStub = decodeAndValidateURLValuesStub(&payloads.ServiceOfferingList{
						IncludeResourceRules: []params.IncludeResourceRule{{
							RelationshipPath: []string{"service_broker"},
							Fields:           []string{"guid"},
						}},
					})
				})

				It("includes broker fields in the response", func() {
					Expect(rr).Should(HaveHTTPStatus(http.StatusOK))
					Expect(rr).To(HaveHTTPBody(SatisfyAll(
						MatchJSONPath("$.included.service_brokers[0].guid", "broker-guid"),
					)))
				})
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

		When("listing the offerings fails", func() {
			BeforeEach(func() {
				serviceOfferingRepo.ListOfferingsReturns(nil, errors.New("list-err"))
			})

			It("returns an error", func() {
				expectUnknownError()
			})
		})
	})

	Describe("DELETE /v3/service_offerings/:guid", func() {
		JustBeforeEach(func() {
			req, err := http.NewRequestWithContext(ctx, "DELETE", "/v3/service_offerings/offering-guid", nil)
			Expect(err).NotTo(HaveOccurred())

			routerBuilder.Build().ServeHTTP(rr, req)
		})

		It("deletes the service offering", func() {
			Expect(serviceOfferingRepo.DeleteOfferingCallCount()).To(Equal(1))
			_, actualAuthInfo, actualDeleteMessage := serviceOfferingRepo.DeleteOfferingArgsForCall(0)
			Expect(actualAuthInfo).To(Equal(authInfo))
			Expect(actualDeleteMessage.GUID).To(Equal("offering-guid"))
			Expect(actualDeleteMessage.Purge).To(BeFalse())

			Expect(rr).To(HaveHTTPStatus(http.StatusNoContent))
		})

		When("deleting the service offering fails with not found", func() {
			BeforeEach(func() {
				serviceOfferingRepo.DeleteOfferingReturns(apierrors.NewNotFoundError(nil, repositories.ServiceOfferingResourceType))
			})

			It("returns 404 Not Found", func() {
				expectNotFoundError("Service Offering")
			})
		})

		When("deleting the service offering fails", func() {
			BeforeEach(func() {
				serviceOfferingRepo.DeleteOfferingReturns(errors.New("boom"))
			})

			It("returns 500 Internal Server Error", func() {
				expectUnknownError()
			})
		})

		When("purging is set to true", func() {
			BeforeEach(func() {
				requestValidator.DecodeAndValidateURLValuesStub = decodeAndValidateURLValuesStub(&payloads.ServiceOfferingDelete{
					Purge: true,
				})
			})

			It("purges the service offering", func() {
				Expect(serviceOfferingRepo.DeleteOfferingCallCount()).To(Equal(1))
				_, actualAuthInfo, actualDeleteMessage := serviceOfferingRepo.DeleteOfferingArgsForCall(0)
				Expect(actualAuthInfo).To(Equal(authInfo))
				Expect(actualDeleteMessage.GUID).To(Equal("offering-guid"))
				Expect(actualDeleteMessage.Purge).To(BeTrue())

				Expect(rr).To(HaveHTTPStatus(http.StatusNoContent))
			})
		})
	})
})
