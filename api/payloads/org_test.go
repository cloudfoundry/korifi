package payloads_test

import (
	"net/http"

	"code.cloudfoundry.org/korifi/api/payloads"
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
			validatorErr = validator.DecodeAndValidateJSONPayload(createRequest(payload), decodedPayload)
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
			validatorErr = validator.DecodeAndValidateJSONPayload(createRequest(payload), decodedPayload)
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
	})

	Describe("OrgList", func() {
		Describe("decoding from url values", func() {
			It("gets the names param and allows order_by", func() {
				orgList := payloads.OrgList{}
				req, err := http.NewRequest("GET", "http://foo.com/bar?names=foo,bar&order_by=name", nil)
				Expect(err).NotTo(HaveOccurred())
				err = validator.DecodeAndValidateURLValues(req, &orgList)

				Expect(err).NotTo(HaveOccurred())
				Expect(orgList.Names).To(Equal("foo,bar"))
			})
		})

		Describe("ToMessage", func() {
			It("splits names to strings", func() {
				orgList := payloads.OrgList{
					Names: "foo,bar",
				}
				Expect(orgList.ToMessage().Names).To(ConsistOf("foo", "bar"))
			})
		})
	})
})
