package payloads_test

import (
	"code.cloudfoundry.org/korifi/api/payloads"
	"code.cloudfoundry.org/korifi/api/repositories"
	"code.cloudfoundry.org/korifi/tools"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gstruct"
)

var _ = Describe("Org", func() {
	Describe("OrgCreate", func() {
		var (
			payload        payloads.OrgCreate
			decodedPayload *payloads.OrgCreate
			validatorErr   error
		)

		BeforeEach(func() {
			payload = payloads.OrgCreate{
				Name:      "my-org",
				Suspended: true,
				Metadata: payloads.Metadata{
					Annotations: map[string]string{
						"foo": "bar",
					},
					Labels: map[string]string{
						"bob": "alice",
					},
				},
			}

			decodedPayload = new(payloads.OrgCreate)
		})

		JustBeforeEach(func() {
			validatorErr = validator.DecodeAndValidateJSONPayload(createJSONRequest(payload), decodedPayload)
		})

		It("succeeds", func() {
			Expect(validatorErr).NotTo(HaveOccurred())
			Expect(decodedPayload).To(gstruct.PointTo(Equal(payload)))
		})

		When("the org name is missing", func() {
			BeforeEach(func() {
				payload.Name = ""
			})

			It("returns a status 422 with appropriate error message json", func() {
				expectUnprocessableEntityError(validatorErr, "name cannot be blank")
			})
		})
	})

	Describe("OrgPatch", func() {
		var (
			payload        payloads.OrgPatch
			decodedPayload *payloads.OrgPatch
			validatorErr   error
		)

		BeforeEach(func() {
			payload = payloads.OrgPatch{
				Name: tools.PtrTo("new-org-name"),
				Metadata: payloads.MetadataPatch{
					Annotations: map[string]*string{
						"foo": tools.PtrTo("bar"),
					},
					Labels: map[string]*string{
						"bob": tools.PtrTo("alice"),
					},
				},
			}

			decodedPayload = new(payloads.OrgPatch)
		})

		JustBeforeEach(func() {
			validatorErr = validator.DecodeAndValidateJSONPayload(createJSONRequest(payload), decodedPayload)
		})

		It("succeeds", func() {
			Expect(validatorErr).NotTo(HaveOccurred())
			Expect(decodedPayload).To(gstruct.PointTo(Equal(payload)))
		})

		When("the metadata is invalid", func() {
			BeforeEach(func() {
				payload.Metadata.Labels["cloudfoundry.org/test"] = tools.PtrTo("production")
			})

			It("returns an unprocessable entity error", func() {
				expectUnprocessableEntityError(
					validatorErr,
					"label/annotation key cannot use the cloudfoundry.org domain",
				)
			})
		})
		When("the name is invalid", func() {
			BeforeEach(func() {
				payload.Name = tools.PtrTo("")
			})

			It("returns an unprocessable entity error", func() {
				expectUnprocessableEntityError(
					validatorErr,
					"name cannot be blank",
				)
			})
		})
	})

	Describe("OrgList", func() {
		DescribeTable("valid query",
			func(query string, expectedOrgList payloads.OrgList) {
				actualOrgList, decodeErr := decodeQuery[payloads.OrgList](query)

				Expect(decodeErr).NotTo(HaveOccurred())
				Expect(*actualOrgList).To(Equal(expectedOrgList))
			},
			Entry("names", "names=o1,o2", payloads.OrgList{Names: "o1,o2"}),
			Entry("created_at", "order_by=created_at", payloads.OrgList{OrderBy: "created_at"}),
			Entry("-created_at", "order_by=-created_at", payloads.OrgList{OrderBy: "-created_at"}),
			Entry("updated_at", "order_by=updated_at", payloads.OrgList{OrderBy: "updated_at"}),
			Entry("-updated_at", "order_by=-updated_at", payloads.OrgList{OrderBy: "-updated_at"}),
			Entry("name", "order_by=name", payloads.OrgList{OrderBy: "name"}),
			Entry("-name", "order_by=-name", payloads.OrgList{OrderBy: "-name"}),
			Entry("pagination", "page=3", payloads.OrgList{Pagination: payloads.Pagination{Page: "3"}}),
		)

		DescribeTable("invalid query",
			func(query string, expectedErrMsg string) {
				_, decodeErr := decodeQuery[payloads.OrgList](query)
				Expect(decodeErr).To(MatchError(ContainSubstring(expectedErrMsg)))
			},
			Entry("invalid parameter", "foo=bar", "unsupported query parameter: foo"),
			Entry("invalid pagination", "per_page=foo", "value must be an integer"),
		)

		Describe("ToMessage", func() {
			It("splits names to strings", func() {
				orgList := payloads.OrgList{
					Names:   "foo,bar",
					OrderBy: "created_at",
					Pagination: payloads.Pagination{
						PerPage: "10",
						Page:    "2",
					},
				}
				Expect(orgList.ToMessage()).To(Equal(repositories.ListOrgsMessage{
					Names:   []string{"foo", "bar"},
					OrderBy: "created_at",
					Pagination: repositories.Pagination{
						PerPage: 10,
						Page:    2,
					},
				}))
			})
		})
	})
})
