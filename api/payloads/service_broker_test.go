package payloads_test

import (
	"code.cloudfoundry.org/korifi/api/payloads"
	"code.cloudfoundry.org/korifi/api/repositories"
	"code.cloudfoundry.org/korifi/model"
	"code.cloudfoundry.org/korifi/model/services"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
)

var _ = Describe("ServiceBrokerCreate", func() {
	var (
		createPayload       payloads.ServiceBrokerCreate
		serviceBrokerCreate *payloads.ServiceBrokerCreate
		validatorErr        error
	)

	BeforeEach(func() {
		serviceBrokerCreate = new(payloads.ServiceBrokerCreate)
		createPayload = payloads.ServiceBrokerCreate{
			Metadata: model.Metadata{
				Labels: map[string]string{
					"label": "label-value",
				},
				Annotations: map[string]string{
					"annotation": "annotation-value",
				},
			},
			ServiceBroker: services.ServiceBroker{
				Name: "my-broker",
				URL:  "https://my.broker.com",
			},
			Authentication: &payloads.BrokerAuthentication{
				Credentials: services.BrokerCredentials{
					Username: "broker-user",
					Password: "broker-password",
				},
				Type: "basic",
			},
		}
	})

	JustBeforeEach(func() {
		validatorErr = validator.DecodeAndValidateJSONPayload(createJSONRequest(createPayload), serviceBrokerCreate)
	})

	It("succeeds", func() {
		Expect(validatorErr).NotTo(HaveOccurred())
		Expect(serviceBrokerCreate).To(PointTo(Equal(createPayload)))
	})

	When("name is not set", func() {
		BeforeEach(func() {
			createPayload.Name = ""
		})

		It("returns an appropriate error", func() {
			expectUnprocessableEntityError(validatorErr, "name cannot be blank")
		})
	})

	When("url is not set", func() {
		BeforeEach(func() {
			createPayload.URL = ""
		})

		It("returns an appropriate error", func() {
			expectUnprocessableEntityError(validatorErr, "url cannot be blank")
		})
	})

	When("authentication is not set", func() {
		BeforeEach(func() {
			createPayload.Authentication = nil
		})

		It("returns an appropriate error", func() {
			expectUnprocessableEntityError(validatorErr, "authentication cannot be blank")
		})
	})

	When("authentication type is invalid", func() {
		BeforeEach(func() {
			createPayload.Authentication.Type = "invalid-auth"
		})

		It("returns an appropriate error", func() {
			expectUnprocessableEntityError(validatorErr, "type value must be one of: basic")
		})
	})

	Describe("ToServiceBrokerCreateMessage()", func() {
		It("converts to repo message correctly", func() {
			msg := serviceBrokerCreate.ToCreateServiceBrokerMessage()
			Expect(msg).To(Equal(repositories.CreateServiceBrokerMessage{
				Metadata: model.Metadata{
					Labels:      map[string]string{"label": "label-value"},
					Annotations: map[string]string{"annotation": "annotation-value"},
				},
				Broker: services.ServiceBroker{
					Name: "my-broker",
					URL:  "https://my.broker.com",
				},
				Credentials: services.BrokerCredentials{
					Username: "broker-user",
					Password: "broker-password",
				},
			}))
		})
	})
})
