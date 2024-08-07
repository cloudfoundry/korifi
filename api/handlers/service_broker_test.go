package handlers_test

import (
	"errors"
	"net/http"
	"strings"

	apierrors "code.cloudfoundry.org/korifi/api/errors"
	"code.cloudfoundry.org/korifi/api/handlers"
	"code.cloudfoundry.org/korifi/api/handlers/fake"
	"code.cloudfoundry.org/korifi/api/payloads"
	"code.cloudfoundry.org/korifi/api/repositories"
	"code.cloudfoundry.org/korifi/model"
	"code.cloudfoundry.org/korifi/model/services"
	. "code.cloudfoundry.org/korifi/tests/matchers"
	"code.cloudfoundry.org/korifi/tools"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("ServiceBroker", func() {
	var (
		serviceBrokerRepo *fake.CFServiceBrokerRepository
		requestValidator  *fake.RequestValidator

		req     *http.Request
		handler *handlers.ServiceBroker
	)

	BeforeEach(func() {
		serviceBrokerRepo = new(fake.CFServiceBrokerRepository)
		requestValidator = new(fake.RequestValidator)
		handler = handlers.NewServiceBroker(
			*serverURL,
			serviceBrokerRepo,
			requestValidator,
		)
	})

	JustBeforeEach(func() {
		routerBuilder.LoadRoutes(handler)
		routerBuilder.Build().ServeHTTP(rr, req)
	})

	Describe("POST /v3/service_brokers", func() {
		var payload payloads.ServiceBrokerCreate

		BeforeEach(func() {
			payload = payloads.ServiceBrokerCreate{
				ServiceBroker: services.ServiceBroker{
					Name: "my-broker",
				},
				Authentication: &payloads.BrokerAuthentication{},
			}
			requestValidator.DecodeAndValidateJSONPayloadStub = decodeAndValidatePayloadStub(&payload)

			serviceBrokerRepo.CreateServiceBrokerReturns(repositories.ServiceBrokerRecord{
				CFResource: model.CFResource{
					GUID: "service-broker-guid",
				},
			}, nil)
			var err error
			req, err = http.NewRequestWithContext(ctx, "POST", "/v3/service_brokers", strings.NewReader("request-body"))
			Expect(err).NotTo(HaveOccurred())
		})

		It("creates a service broker", func() {
			Expect(requestValidator.DecodeAndValidateJSONPayloadCallCount()).To(Equal(1))
			actualReq, _ := requestValidator.DecodeAndValidateJSONPayloadArgsForCall(0)
			Expect(bodyString(actualReq)).To(Equal("request-body"))

			Expect(serviceBrokerRepo.CreateServiceBrokerCallCount()).To(Equal(1))
			_, actualAuthInfo, actualCreateMsg := serviceBrokerRepo.CreateServiceBrokerArgsForCall(0)
			Expect(actualAuthInfo).To(Equal(authInfo))
			Expect(actualCreateMsg.Broker.Name).To(Equal("my-broker"))

			Expect(rr).To(HaveHTTPStatus(http.StatusAccepted))
			Expect(rr).To(HaveHTTPHeaderWithValue("Location", "https://api.example.org/v3/jobs/service_broker.create~service-broker-guid"))
		})

		When("the request body is invalid json", func() {
			BeforeEach(func() {
				requestValidator.DecodeAndValidateJSONPayloadReturns(errors.New("boom"))
			})

			It("returns an error", func() {
				expectUnknownError()
			})
		})

		When("creating the service broker fails", func() {
			BeforeEach(func() {
				serviceBrokerRepo.CreateServiceBrokerReturns(repositories.ServiceBrokerRecord{}, errors.New("create-service-broker-error"))
			})

			It("returns an error", func() {
				expectUnknownError()
			})
		})
	})

	Describe("GET /v3/service_brokers", func() {
		BeforeEach(func() {
			serviceBrokerRepo.ListServiceBrokersReturns([]repositories.ServiceBrokerRecord{
				{
					CFResource: model.CFResource{
						GUID: "broker-guid",
					},
				},
			}, nil)

			var err error
			req, err = http.NewRequestWithContext(ctx, "GET", "/v3/service_brokers", nil)
			Expect(err).NotTo(HaveOccurred())
		})

		It("lists the service brokers", func() {
			Expect(serviceBrokerRepo.ListServiceBrokersCallCount()).To(Equal(1))
			_, actualAuthInfo, actualListMsg := serviceBrokerRepo.ListServiceBrokersArgsForCall(0)
			Expect(actualAuthInfo).To(Equal(authInfo))
			Expect(actualListMsg).To(Equal(repositories.ListServiceBrokerMessage{}))

			Expect(rr).Should(HaveHTTPStatus(http.StatusOK))
			Expect(rr).To(HaveHTTPHeaderWithValue("Content-Type", "application/json"))
			Expect(rr).To(HaveHTTPBody(SatisfyAll(
				MatchJSONPath("$.pagination.total_results", BeEquivalentTo(1)),
				MatchJSONPath("$.pagination.first.href", "https://api.example.org/v3/service_brokers"),
				MatchJSONPath("$.resources[0].guid", "broker-guid"),
				MatchJSONPath("$.resources[0].links.self.href", "https://api.example.org/v3/service_brokers/broker-guid"),
			)))
		})

		When("filtering query params are provided", func() {
			BeforeEach(func() {
				requestValidator.DecodeAndValidateURLValuesStub = decodeAndValidateURLValuesStub(&payloads.ServiceBrokerList{
					Names: "b1,b2",
				})
			})

			It("passes them to the repository", func() {
				Expect(serviceBrokerRepo.ListServiceBrokersCallCount()).To(Equal(1))
				_, _, message := serviceBrokerRepo.ListServiceBrokersArgsForCall(0)

				Expect(message.Names).To(ConsistOf("b1", "b2"))
			})
		})

		When("decoding query parameters fails", func() {
			BeforeEach(func() {
				requestValidator.DecodeAndValidateURLValuesReturns(errors.New("decode-err"))
			})

			It("returns an error", func() {
				expectUnknownError()
			})
		})

		When("listing service brokers fails", func() {
			BeforeEach(func() {
				serviceBrokerRepo.ListServiceBrokersReturns(nil, errors.New("list-brokers-error"))
			})

			It("returns an error", func() {
				expectUnknownError()
			})
		})
	})

	Describe("DELETE /v3/service_brokers/guid", func() {
		BeforeEach(func() {
			serviceBrokerRepo.GetServiceBrokerReturns(repositories.ServiceBrokerRecord{
				CFResource: model.CFResource{
					GUID: "broker-guid",
				},
			}, nil)

			var err error
			req, err = http.NewRequestWithContext(ctx, "DELETE", "/v3/service_brokers/broker-guid", nil)
			Expect(err).NotTo(HaveOccurred())
		})

		It("deletes the service broker", func() {
			Expect(serviceBrokerRepo.GetServiceBrokerCallCount()).To(Equal(1))
			_, actualAuthInfo, actualBrokerGUID := serviceBrokerRepo.GetServiceBrokerArgsForCall(0)
			Expect(actualAuthInfo).To(Equal(authInfo))
			Expect(actualBrokerGUID).To(Equal("broker-guid"))

			Expect(serviceBrokerRepo.DeleteServiceBrokerCallCount()).To(Equal(1))
			_, actualAuthInfo, actualBrokerGUID = serviceBrokerRepo.DeleteServiceBrokerArgsForCall(0)
			Expect(actualAuthInfo).To(Equal(authInfo))
			Expect(actualBrokerGUID).To(Equal("broker-guid"))

			Expect(rr).To(HaveHTTPStatus(http.StatusAccepted))
			Expect(rr).To(HaveHTTPHeaderWithValue("Location", "https://api.example.org/v3/jobs/service_broker.delete~broker-guid"))
		})

		When("getting the service broker is not allowed", func() {
			BeforeEach(func() {
				serviceBrokerRepo.GetServiceBrokerReturns(repositories.ServiceBrokerRecord{}, apierrors.NewForbiddenError(nil, repositories.ServiceBrokerResourceType))
			})

			It("returns a not found error", func() {
				expectNotFoundError(repositories.ServiceBrokerResourceType)
			})
		})

		When("getting the service broker fails", func() {
			BeforeEach(func() {
				serviceBrokerRepo.GetServiceBrokerReturns(repositories.ServiceBrokerRecord{}, errors.New("get-broker-err"))
			})

			It("returns an error", func() {
				expectUnknownError()
			})
		})

		When("getting the service broker fails", func() {
			BeforeEach(func() {
				serviceBrokerRepo.DeleteServiceBrokerReturns(errors.New("delete-broker-err"))
			})

			It("returns an error", func() {
				expectUnknownError()
			})
		})
	})

	Describe("PATCH /v3/service_brokers", func() {
		BeforeEach(func() {
			requestValidator.DecodeAndValidateJSONPayloadStub = decodeAndValidatePayloadStub(&payloads.ServiceBrokerUpdate{
				Name: tools.PtrTo("new-name"),
			})

			serviceBrokerRepo.UpdateServiceBrokerReturns(repositories.ServiceBrokerRecord{
				CFResource: model.CFResource{
					GUID: "service-broker-guid",
				},
			}, nil)

			var err error
			req, err = http.NewRequestWithContext(ctx, "PATCH", "/v3/service_brokers/service-broker-guid", strings.NewReader("the-json-body"))
			Expect(err).NotTo(HaveOccurred())
		})

		It("updates the service broker", func() {
			Expect(requestValidator.DecodeAndValidateJSONPayloadCallCount()).To(Equal(1))
			actualReq, _ := requestValidator.DecodeAndValidateJSONPayloadArgsForCall(0)
			Expect(bodyString(actualReq)).To(Equal("the-json-body"))

			Expect(serviceBrokerRepo.UpdateServiceBrokerCallCount()).To(Equal(1))
			_, actualAuthInfo, actualUpdateMesage := serviceBrokerRepo.UpdateServiceBrokerArgsForCall(0)
			Expect(actualAuthInfo).To(Equal(authInfo))
			Expect(actualUpdateMesage).To(Equal(repositories.UpdateServiceBrokerMessage{
				GUID: "service-broker-guid",
				Name: tools.PtrTo("new-name"),
			}))

			Expect(rr).To(HaveHTTPStatus(http.StatusAccepted))
			Expect(rr).To(HaveHTTPHeaderWithValue("Location", "https://api.example.org/v3/jobs/service_broker.update~service-broker-guid"))
		})

		When("only metadata is updated", func() {
			BeforeEach(func() {
				requestValidator.DecodeAndValidateJSONPayloadStub = decodeAndValidatePayloadStub(&payloads.ServiceBrokerUpdate{
					Metadata: payloads.MetadataPatch{
						Labels: map[string]*string{
							"foo": tools.PtrTo("bar"),
						},
					},
				})
				serviceBrokerRepo.UpdateServiceBrokerReturns(repositories.ServiceBrokerRecord{
					CFResource: model.CFResource{
						GUID: "service-broker-guid",
						Metadata: model.Metadata{
							Labels: map[string]string{
								"foo": "bar",
							},
						},
					},
				}, nil)
			})

			It("returns OK response", func() {
				Expect(rr).To(HaveHTTPStatus(http.StatusOK))
				Expect(rr).To(HaveHTTPHeaderWithValue("Content-Type", "application/json"))
				Expect(rr).To(HaveHTTPBody(SatisfyAll(
					MatchJSONPath("$.guid", "service-broker-guid"),
					MatchJSONPath("$.metadata.labels.foo", "bar"),
					MatchJSONPath("$.links.self.href", "https://api.example.org/v3/service_brokers/service-broker-guid"),
				)))
			})
		})

		When("the user doesn't have permission to get the broker", func() {
			BeforeEach(func() {
				serviceBrokerRepo.GetServiceBrokerReturns(repositories.ServiceBrokerRecord{}, apierrors.NewForbiddenError(nil, repositories.ServiceBrokerResourceType))
			})

			It("returns a not found error", func() {
				expectNotFoundError(repositories.ServiceBrokerResourceType)
			})

			It("does not call update", func() {
				Expect(serviceBrokerRepo.UpdateServiceBrokerCallCount()).To(Equal(0))
			})
		})

		When("fetching the broker errors", func() {
			BeforeEach(func() {
				serviceBrokerRepo.GetServiceBrokerReturns(repositories.ServiceBrokerRecord{}, errors.New("get-broker-err"))
			})

			It("returns an error", func() {
				expectUnknownError()
			})

			It("does not call update", func() {
				Expect(serviceBrokerRepo.UpdateServiceBrokerCallCount()).To(Equal(0))
			})
		})

		When("updating the broker fails", func() {
			BeforeEach(func() {
				serviceBrokerRepo.UpdateServiceBrokerReturns(repositories.ServiceBrokerRecord{}, errors.New("update-err"))
			})

			It("returns an error", func() {
				expectUnknownError()
			})
		})

		When("the request body is invalid", func() {
			BeforeEach(func() {
				requestValidator.DecodeAndValidateJSONPayloadReturns(apierrors.NewUnprocessableEntityError(errors.New("validation-err"), "validation error"))
			})

			It("returns an unprocessable entity error", func() {
				expectUnprocessableEntityError("validation error")
			})
		})
	})
})
