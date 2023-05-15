package payloads_test

import (
	"net/url"

	"code.cloudfoundry.org/korifi/api/payloads"
	"code.cloudfoundry.org/korifi/api/repositories"
	"code.cloudfoundry.org/korifi/tools"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("DomainCreate", func() {
	Describe("Decode", func() {
		var (
			createPayload *payloads.DomainCreate
			validatorErr  error
		)

		BeforeEach(func() {
			createPayload = &payloads.DomainCreate{
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
			validatorErr = validator.ValidatePayload(createPayload)
		})

		It("succeeds", func() {
			Expect(validatorErr).NotTo(HaveOccurred())
		})

		When("name is empty", func() {
			BeforeEach(func() {
				createPayload.Name = ""
			})

			It("returns an appropriate error", func() {
				expectUnprocessableEntityError(validatorErr, "Name is a required field")
			})
		})

		When("metadata.labels contains an invalid key", func() {
			BeforeEach(func() {
				createPayload.Metadata = payloads.Metadata{
					Labels: map[string]string{
						"foo.cloudfoundry.org/bar": "jim",
					},
				}
			})

			It("returns an appropriate error", func() {
				expectUnprocessableEntityError(validatorErr, "cannot begin with \"cloudfoundry.org\"")
			})
		})

		When("metadata.annotations contains an invalid key", func() {
			BeforeEach(func() {
				createPayload.Metadata = payloads.Metadata{
					Annotations: map[string]string{
						"foo.cloudfoundry.org/bar": "jim",
					},
				}
			})

			It("returns an appropriate error", func() {
				expectUnprocessableEntityError(validatorErr, "cannot begin with \"cloudfoundry.org\"")
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
		updatePayload *payloads.DomainUpdate
		validatorErr  error
	)

	BeforeEach(func() {
		updatePayload = &payloads.DomainUpdate{
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
		validatorErr = validator.ValidatePayload(updatePayload)
	})

	It("succeeds", func() {
		Expect(validatorErr).NotTo(HaveOccurred())
	})

	When("metadata.labels contains an invalid key", func() {
		BeforeEach(func() {
			updatePayload.Metadata = payloads.MetadataPatch{
				Labels: map[string]*string{
					"foo.cloudfoundry.org/bar": tools.PtrTo("jim"),
				},
			}
		})

		It("returns an appropriate error", func() {
			expectUnprocessableEntityError(validatorErr, "cannot begin with \"cloudfoundry.org\"")
		})
	})

	When("metadata.annotations contains an invalid key", func() {
		BeforeEach(func() {
			updatePayload.Metadata = payloads.MetadataPatch{
				Annotations: map[string]*string{
					"foo.cloudfoundry.org/bar": tools.PtrTo("jim"),
				},
			}
		})

		It("returns an appropriate error", func() {
			expectUnprocessableEntityError(validatorErr, "cannot begin with \"cloudfoundry.org\"")
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
