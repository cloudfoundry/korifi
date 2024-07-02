package handlers_test

import (
	"errors"
	"net/http"
	"strings"

	"code.cloudfoundry.org/korifi/api/handlers"
	"code.cloudfoundry.org/korifi/api/handlers/fake"
	"code.cloudfoundry.org/korifi/api/payloads"
	"code.cloudfoundry.org/korifi/api/repositories"
	"code.cloudfoundry.org/korifi/model"
	"code.cloudfoundry.org/korifi/model/services"

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
			true,
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

			serviceBrokerRepo.CreateServiceBrokerReturns(repositories.ServiceBrokerResource{
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
				serviceBrokerRepo.CreateServiceBrokerReturns(repositories.ServiceBrokerResource{}, errors.New("create-service-broker-error"))
			})

			It("returns an error", func() {
				expectUnknownError()
			})
		})

		When("managed services are not enabled", func() {
			BeforeEach(func() {
				handler = handlers.NewServiceBroker(
					*serverURL,
					serviceBrokerRepo,
					requestValidator,
					false,
				)
			})

			It("returns unprocessable entity error", func() {
				expectUnprocessableEntityError("Managed services are not enabled")
			})
		})
	})
})
