package payloads_test

import (
	"code.cloudfoundry.org/korifi/api/payloads"
	"code.cloudfoundry.org/korifi/api/repositories"
	"code.cloudfoundry.org/korifi/tools"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	"github.com/onsi/gomega/types"
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
			Labels: map[string]string{
				"label": "label-value",
			},
			Annotations: map[string]string{
				"annotation": "annotation-value",
			},
			Name: "my-broker",
			URL:  "https://my.broker.com",
			Authentication: &payloads.BrokerAuthentication{
				Type: "basic",
				Credentials: payloads.BrokerCredentials{
					Username: "broker-user",
					Password: "broker-password",
				},
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
				Metadata: repositories.Metadata{
					Labels:      map[string]string{"label": "label-value"},
					Annotations: map[string]string{"annotation": "annotation-value"},
				},
				Name: "my-broker",
				URL:  "https://my.broker.com",
				Credentials: repositories.BrokerCredentials{
					Username: "broker-user",
					Password: "broker-password",
				},
			}))
		})
	})
})

var _ = Describe("ServiceBrokerList", func() {
	DescribeTable("valid query",
		func(query string, expectedServiceBrokerList payloads.ServiceBrokerList) {
			actualServiceBrokerList, decodeErr := decodeQuery[payloads.ServiceBrokerList](query)

			Expect(decodeErr).NotTo(HaveOccurred())
			Expect(*actualServiceBrokerList).To(Equal(expectedServiceBrokerList))
		},
		Entry("names", "names=n1,n2", payloads.ServiceBrokerList{Names: "n1,n2"}),
		Entry("page=3", "page=3", payloads.ServiceBrokerList{Pagination: payloads.Pagination{Page: "3"}}),
	)

	DescribeTable("invalid query",
		func(query string, errMatcher types.GomegaMatcher) {
			_, decodeErr := decodeQuery[payloads.ServiceBrokerList](query)
			Expect(decodeErr).To(errMatcher)
		},
		Entry("per_page is not a number", "per_page=foo", MatchError(ContainSubstring("value must be an integer"))),
	)

	Describe("ToMessage", func() {
		var (
			payload payloads.ServiceBrokerList
			message repositories.ListServiceBrokerMessage
		)

		BeforeEach(func() {
			payload = payloads.ServiceBrokerList{
				Names: "n1,n2",
				Pagination: payloads.Pagination{
					PerPage: "3",
					Page:    "4",
				},
			}
		})

		JustBeforeEach(func() {
			message = payload.ToMessage()
		})

		It("returns a list service bindings message", func() {
			Expect(message).To(Equal(repositories.ListServiceBrokerMessage{
				Names: []string{"n1", "n2"},
				Pagination: repositories.Pagination{
					PerPage: 3,
					Page:    4,
				},
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
					Type: "basic",
					Credentials: payloads.BrokerCredentials{
						Username: "user",
						Password: "pass",
					},
				}
			})

			It("converts to repo message with credentials", func() {
				Expect(updatePayload.ToMessage("broker-guid").Credentials).To(Equal(&repositories.BrokerCredentials{
					Username: "user",
					Password: "pass",
				}))
			})
		})
	})
})
