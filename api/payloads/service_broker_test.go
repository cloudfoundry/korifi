package payloads_test

import (
	"net/http"

	"code.cloudfoundry.org/korifi/api/payloads"
	"code.cloudfoundry.org/korifi/api/repositories"
	"code.cloudfoundry.org/korifi/model"
	"code.cloudfoundry.org/korifi/model/services"
	"code.cloudfoundry.org/korifi/tools"
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

	Describe("ToMessage()", func() {
		It("converts to repo message correctly", func() {
			msg := serviceBrokerCreate.ToMessage()
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

var _ = Describe("ServiceBrokerList", func() {
	var serviceBrokerList payloads.ServiceBrokerList

	BeforeEach(func() {
		serviceBrokerList = payloads.ServiceBrokerList{
			Names: "b1, b2",
		}
	})

	Describe("decodes from url values", func() {
		It("succeeds", func() {
			req, err := http.NewRequest("GET", "http://foo.com/bar?names=foo,bar", nil)
			Expect(err).NotTo(HaveOccurred())
			err = validator.DecodeAndValidateURLValues(req, &serviceBrokerList)

			Expect(err).NotTo(HaveOccurred())
			Expect(serviceBrokerList.Names).To(Equal("foo,bar"))
		})
	})

	Describe("ToMessage", func() {
		It("converts to repo message correctly", func() {
			Expect(serviceBrokerList.ToMessage()).To(Equal(repositories.ListServiceBrokerMessage{
				Names: []string{"b1", "b2"},
			}))
		})
	})
})

var _ = Describe("ServiceBrokerUpdate", func() {
	var updatePayload payloads.ServiceBrokerUpdate

	BeforeEach(func() {
		updatePayload = payloads.ServiceBrokerUpdate{
			Name: tools.PtrTo("my-broker"),
			URL:  tools.PtrTo("my-broker-url"),
			Metadata: payloads.MetadataPatch{
				Labels: map[string]*string{
					"foo": tools.PtrTo("bar"),
				},
				Annotations: map[string]*string{
					"baz": tools.PtrTo("qux"),
				},
			},
		}
	})

	Describe("validate", func() {
		var (
			validatorErr   error
			decodedRequest payloads.ServiceBrokerUpdate
		)

		JustBeforeEach(func() {
			validatorErr = validator.DecodeAndValidateJSONPayload(createJSONRequest(updatePayload), &decodedRequest)
		})

		It("succeeds", func() {
			Expect(validatorErr).NotTo(HaveOccurred())
			Expect(decodedRequest).To(Equal(updatePayload))
		})

		When("authentication type is invalid", func() {
			BeforeEach(func() {
				updatePayload.Authentication = &payloads.BrokerAuthentication{
					Type: "invalid-auth",
				}
			})

			It("returns an appropriate error", func() {
				expectUnprocessableEntityError(validatorErr, "type value must be one of: basic")
			})
		})
	})

	DescribeTable("IsAsyncRequest",
		func(payload payloads.ServiceBrokerUpdate, expectedIsAsync bool) {
			Expect(payload.IsAsyncRequest()).To(Equal(expectedIsAsync))
		},
		Entry("name", payloads.ServiceBrokerUpdate{Name: tools.PtrTo("foo")}, true),
		Entry("url", payloads.ServiceBrokerUpdate{URL: tools.PtrTo("foo")}, true),
		Entry("authentication", payloads.ServiceBrokerUpdate{Authentication: &payloads.BrokerAuthentication{}}, true),
		Entry("metadata", payloads.ServiceBrokerUpdate{}, false),
	)

	Describe("ToMessage", func() {
		It("converts to repo message", func() {
			Expect(updatePayload.ToMessage("broker-guid")).To(Equal(repositories.UpdateServiceBrokerMessage{
				GUID: "broker-guid",
				Name: tools.PtrTo("my-broker"),
				URL:  tools.PtrTo("my-broker-url"),
				MetadataPatch: repositories.MetadataPatch{
					Labels: map[string]*string{
						"foo": tools.PtrTo("bar"),
					},
					Annotations: map[string]*string{
						"baz": tools.PtrTo("qux"),
					},
				},
			}))
		})

		When("the message has cretentials", func() {
			BeforeEach(func() {
				updatePayload.Authentication = &payloads.BrokerAuthentication{
					Credentials: services.BrokerCredentials{
						Username: "user",
						Password: "pass",
					},
					Type: "basic",
				}
			})

			It("converts to repo message with credentials", func() {
				Expect(updatePayload.ToMessage("broker-guid").Credentials).To(Equal(&services.BrokerCredentials{
					Username: "user",
					Password: "pass",
				}))
			})
		})
	})
})
