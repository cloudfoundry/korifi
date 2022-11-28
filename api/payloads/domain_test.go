package payloads_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/url"

	"code.cloudfoundry.org/korifi/api/handlers"
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
			validator            *handlers.DecoderValidator
			validatorErr         error
		)

		BeforeEach(func() {
			var err error
			validator, err = handlers.NewDefaultDecoderValidator()
			Expect(err).NotTo(HaveOccurred())

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
			body, err := json.Marshal(createPayload)
			Expect(err).NotTo(HaveOccurred())

			req, err := http.NewRequest("", "", bytes.NewReader(body))
			Expect(err).NotTo(HaveOccurred())

			validatorErr = validator.DecodeAndValidateJSONPayload(req, decodedDomainPayload)
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
		updatePayload        payloads.DomainUpdate
		decodedUpdatePayload *payloads.DomainUpdate
		validator            *handlers.DecoderValidator
		validatorErr         error
	)

	BeforeEach(func() {
		var err error
		validator, err = handlers.NewDefaultDecoderValidator()
		Expect(err).NotTo(HaveOccurred())

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
		updateBody, err := json.Marshal(updatePayload)
		Expect(err).NotTo(HaveOccurred())

		req, err := http.NewRequest("", "", bytes.NewReader(updateBody))
		Expect(err).NotTo(HaveOccurred())

		validatorErr = validator.DecodeAndValidateJSONPayload(req, decodedUpdatePayload)
	})

	It("succeeds", func() {
		Expect(validatorErr).NotTo(HaveOccurred())
		Expect(decodedUpdatePayload).To(gstruct.PointTo(Equal(updatePayload)))
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
	var payload payloads.DomainList

	Describe("Decode", func() {
		var (
			form      url.Values
			decodeErr error
		)

		BeforeEach(func() {
			payload = payloads.DomainList{}
			form = url.Values{}
		})

		JustBeforeEach(func() {
			decodeErr = payloads.Decode(&payload, form)
		})

		It("succeeds", func() {
			Expect(decodeErr).NotTo(HaveOccurred())
			Expect(payload.Names).To(BeNil())
		})

		When("the form has valid keys", func() {
			BeforeEach(func() {
				form = url.Values{
					"names": []string{"foo,bar"},
				}
			})

			It("succeeds", func() {
				Expect(decodeErr).NotTo(HaveOccurred())
				Expect(payload.Names).To(gstruct.PointTo(Equal("foo,bar")))
			})
		})

		When("the form is invalid", func() {
			BeforeEach(func() {
				form = url.Values{
					"bananas": []string{"foo", "bar"},
				}
			})

			It("errors", func() {
				expectUnknownKeyError(decodeErr, "The query parameter is invalid")
			})
		})
	})

	Describe("ToMessage", func() {
		var listDomainsMessage repositories.ListDomainsMessage

		BeforeEach(func() {
			payload = payloads.DomainList{
				Names: tools.PtrTo("foo,bar"),
			}
		})

		JustBeforeEach(func() {
			listDomainsMessage = payload.ToMessage()
		})

		It("splits names to strings", func() {
			Expect(listDomainsMessage.Names).To(ConsistOf("foo", "bar"))
		})
	})
})
