package handlers_test

import (
	"errors"
	"net/http"

	. "code.cloudfoundry.org/korifi/api/handlers"
	"code.cloudfoundry.org/korifi/api/handlers/fake"
	"code.cloudfoundry.org/korifi/api/payloads"
	"code.cloudfoundry.org/korifi/api/repositories"
	"code.cloudfoundry.org/korifi/model"
	"code.cloudfoundry.org/korifi/model/services"
	. "code.cloudfoundry.org/korifi/tests/matchers"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("ServiceOffering", func() {
	var (
		requestValidator    *fake.RequestValidator
		serviceOfferingRepo *fake.CFServiceOfferingRepository
		serviceBrokerRepo   *fake.CFServiceBrokerRepository
	)

	BeforeEach(func() {
		requestValidator = new(fake.RequestValidator)
		serviceOfferingRepo = new(fake.CFServiceOfferingRepository)
		serviceBrokerRepo = new(fake.CFServiceBrokerRepository)

		apiHandler := NewServiceOffering(
			*serverURL,
			requestValidator,
			serviceOfferingRepo,
			serviceBrokerRepo,
		)
		routerBuilder.LoadRoutes(apiHandler)
	})

	Describe("GET /v3/service_offerings", func() {
		BeforeEach(func() {
			serviceOfferingRepo.ListOfferingsReturns([]repositories.ServiceOfferingRecord{{
				ServiceOffering: services.ServiceOffering{},
				CFResource: model.CFResource{
					GUID: "offering-guid",
				},
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
					ServiceBroker: services.ServiceBroker{
						Name: "broker-name",
					},
					CFResource: model.CFResource{
						GUID: "broker-guid",
					},
				}}, nil)

				requestValidator.DecodeAndValidateURLValuesStub = decodeAndValidateURLValuesStub(&payloads.ServiceOfferingList{
					IncludeBrokerFields: []string{"foo"},
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
						IncludeBrokerFields: []string{"name"},
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
						IncludeBrokerFields: []string{"guid"},
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
})
