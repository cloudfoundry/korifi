package payloads_test

import (
	"net/url"

	"code.cloudfoundry.org/korifi/api/payloads"
	"code.cloudfoundry.org/korifi/api/repositories"
	"code.cloudfoundry.org/korifi/tools"
	"github.com/onsi/gomega/gstruct"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("DomainCreate", func() {
	Describe("Decode", func() {
		var (
			createPayload        payloads.DomainCreate
			decodedDomainPayload *payloads.DomainCreate
			validatorErr         error
		)

		BeforeEach(func() {
			decodedDomainPayload = new(payloads.DomainCreate)
			createPayload = payloads.DomainCreate{
				Name: "bob.com",
				Metadata: payloads.Metadata{
					Labels: map[string]string{
						"foo": "bar",
					},
					Annotations: map[string]string{
						"example.org/jim": "hello",
					},
				},
			}
		})

		JustBeforeEach(func() {
			validatorErr = validator.DecodeAndValidateJSONPayload(createRequest(createPayload), decodedDomainPayload)
		})

		It("succeeds", func() {
			Expect(validatorErr).NotTo(HaveOccurred())
			Expect(decodedDomainPayload).To(gstruct.PointTo(Equal(createPayload)))
		})

		When("name is empty", func() {
			BeforeEach(func() {
				createPayload.Name = ""
			})

			It("returns an appropriate error", func() {
				expectUnprocessableEntityError(validatorErr, "name cannot be blank")
			})
		})

		When("metadata is invalid", func() {
			BeforeEach(func() {
				createPayload.Metadata = payloads.Metadata{
					Labels: map[string]string{
						"foo.cloudfoundry.org/bar": "jim",
					},
				}
			})

			It("returns an appropriate error", func() {
				expectUnprocessableEntityError(validatorErr, "cannot use the cloudfoundry.org domain")
			})
		})

		When("relationship is invalid", func() {
			BeforeEach(func() {
				createPayload.Relationships = map[string]payloads.Relationship{
					"foo": {Data: nil},
				}
			})

			It("returns an appropriate error", func() {
				expectUnprocessableEntityError(validatorErr, "data cannot be blank")
			})
		})
	})

	Describe("ToMessage", func() {
		var (
			createPayload payloads.DomainCreate
			createMessage repositories.CreateDomainMessage
			err           error
		)

		BeforeEach(func() {
			createPayload = payloads.DomainCreate{
				Name: "foo.com",
				Metadata: payloads.Metadata{
					Labels:      map[string]string{"foo": "bar"},
					Annotations: map[string]string{"foo": "bar"},
				},
			}
		})

		JustBeforeEach(func() {
			createMessage, err = createPayload.ToMessage()
		})

		It("returns a domain create message", func() {
			Expect(err).NotTo(HaveOccurred())
			Expect(createMessage).To(Equal(repositories.CreateDomainMessage{
				Name: "foo.com",
				Metadata: repositories.Metadata{
					Labels:      map[string]string{"foo": "bar"},
					Annotations: map[string]string{"foo": "bar"},
				},
			}))
		})

		When("the payload has internal set to true", func() {
			BeforeEach(func() {
				createPayload.Internal = true
			})

			It("errors", func() {
				Expect(err).To(MatchError(ContainSubstring("internal domains are not supported")))
			})
		})

		When("the payload has relationships", func() {
			BeforeEach(func() {
				createPayload.Relationships = map[string]payloads.Relationship{
					"foo": {},
				}
			})

			It("errors", func() {
				Expect(err).To(MatchError(ContainSubstring("private domains are not supported")))
			})
		})
	})
})

var _ = Describe("DomainUpdate", func() {
	var (
		updatePayload        payloads.DomainUpdate
		decodedUpdatePayload *payloads.DomainUpdate
		validatorErr         error
	)

	BeforeEach(func() {
		decodedUpdatePayload = new(payloads.DomainUpdate)

		updatePayload = payloads.DomainUpdate{
			Metadata: payloads.MetadataPatch{
				Labels: map[string]*string{
					"foo": tools.PtrTo("bar"),
				},
				Annotations: map[string]*string{
					"example.org/jim": tools.PtrTo("hello"),
				},
			},
		}
	})

	JustBeforeEach(func() {
		validatorErr = validator.DecodeAndValidateJSONPayload(createRequest(updatePayload), decodedUpdatePayload)
	})

	It("succeeds", func() {
		Expect(validatorErr).NotTo(HaveOccurred())
		Expect(decodedUpdatePayload).To(gstruct.PointTo(Equal(updatePayload)))
	})

	When("metadata is invalid", func() {
		BeforeEach(func() {
			updatePayload.Metadata = payloads.MetadataPatch{
				Labels: map[string]*string{
					"foo.cloudfoundry.org/bar": tools.PtrTo("jim"),
				},
			}
		})

		It("returns an appropriate error", func() {
			expectUnprocessableEntityError(validatorErr, "cannot use the cloudfoundry.org domain")
		})
	})
})

var _ = Describe("DomainList", func() {
	Describe("DecodeFromURLValues", func() {
		It("succeeds", func() {
			domainList := payloads.DomainList{}
			err := domainList.DecodeFromURLValues(url.Values{
				"names": []string{"foo,bar"},
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(domainList.Names).To(Equal("foo,bar"))
		})
	})

	Describe("ToMessage", func() {
		It("splits names to strings", func() {
			domainList := payloads.DomainList{
				Names: "foo,bar",
			}
			Expect(domainList.ToMessage().Names).To(ConsistOf("foo", "bar"))
		})
	})
})
