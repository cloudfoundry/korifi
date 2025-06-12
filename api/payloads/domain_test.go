package payloads_test

import (
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
			validatorErr = validator.DecodeAndValidateJSONPayload(createJSONRequest(createPayload), decodedDomainPayload)
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
				expectUnprocessableEntityError(validatorErr, "data is required")
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
		validatorErr = validator.DecodeAndValidateJSONPayload(createJSONRequest(updatePayload), decodedUpdatePayload)
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
	Describe("Validation", func() {
		DescribeTable("valid query",
			func(query string, expectedDomainList payloads.DomainList) {
				actualDomainList, decodeErr := decodeQuery[payloads.DomainList](query)

				Expect(decodeErr).NotTo(HaveOccurred())
				Expect(*actualDomainList).To(Equal(expectedDomainList))
			},

			Entry("names", "names=name", payloads.DomainList{Names: "name"}),
			Entry("order_by created_at", "order_by=created_at", payloads.DomainList{OrderBy: "created_at"}),
			Entry("order_by -created_at", "order_by=-created_at", payloads.DomainList{OrderBy: "-created_at"}),
			Entry("order_by updated_at", "order_by=updated_at", payloads.DomainList{OrderBy: "updated_at"}),
			Entry("order_by -updated_at", "order_by=-updated_at", payloads.DomainList{OrderBy: "-updated_at"}),
			Entry("page=3", "page=3", payloads.DomainList{Pagination: payloads.Pagination{Page: "3"}}),
		)

		DescribeTable("invalid query",
			func(query string, expectedErrMsg string) {
				_, decodeErr := decodeQuery[payloads.DomainList](query)
				Expect(decodeErr).To(MatchError(ContainSubstring(expectedErrMsg)))
			},
			Entry("invalid order_by", "order_by=foo", "value must be one of"),
			Entry("per_page is not a number", "per_page=foo", "value must be an integer"),
		)
	})

	Describe("ToMessage", func() {
		It("translates to repo message", func() {
			domainList := payloads.DomainList{
				Names:   "foo,bar",
				OrderBy: "created_at",
				Pagination: payloads.Pagination{
					PerPage: "3",
					Page:    "2",
				},
			}
			Expect(domainList.ToMessage()).To(Equal(repositories.ListDomainsMessage{
				Names:   []string{"foo", "bar"},
				OrderBy: "created_at",
				Pagination: repositories.Pagination{
					Page:    2,
					PerPage: 3,
				},
			}))
		})
	})
})
