package payloads_test

import (
	"net/http"

	"code.cloudfoundry.org/korifi/api/payloads"
	"code.cloudfoundry.org/korifi/tools"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gstruct"
)

var _ = Describe("Space", func() {
	Describe("SpaceCreate", func() {
		var (
			payload        payloads.SpaceCreate
			decodedPayload *payloads.SpaceCreate
			validatorErr   error
		)

		BeforeEach(func() {
			payload = payloads.SpaceCreate{
				Name: "my-space",
				Relationships: &payloads.SpaceRelationships{
					Org: &payloads.Relationship{
						Data: &payloads.RelationshipData{
							GUID: "org-guid",
						},
					},
				},
				Metadata: payloads.Metadata{
					Annotations: map[string]string{
						"foo": "bar",
					},
					Labels: map[string]string{
						"bob": "alice",
					},
				},
			}

			decodedPayload = new(payloads.SpaceCreate)
		})

		JustBeforeEach(func() {
			validatorErr = validator.DecodeAndValidateJSONPayload(createRequest(payload), decodedPayload)
		})

		It("succeeds", func() {
			Expect(validatorErr).NotTo(HaveOccurred())
			Expect(decodedPayload).To(gstruct.PointTo(Equal(payload)))
		})

		When("the space name is missing", func() {
			BeforeEach(func() {
				payload.Name = ""
			})

			It("returns a status 422 with appropriate error message json", func() {
				expectUnprocessableEntityError(validatorErr, "name cannot be blank")
			})
		})

		When("relationships is not set", func() {
			BeforeEach(func() {
				payload.Relationships = nil
			})

			It("returns a status 422 with appropriate error message json", func() {
				expectUnprocessableEntityError(validatorErr, "relationships is required")
			})
		})

		When("relationships.organization is not set", func() {
			BeforeEach(func() {
				payload.Relationships.Org = nil
			})

			It("returns a status 422 with appropriate error message json", func() {
				expectUnprocessableEntityError(validatorErr, "relationships.organization is required")
			})
		})

		When("metadata is invalid", func() {
			BeforeEach(func() {
				payload.Metadata.Labels["foo.cloudfoundry.org/bar"] = "baz"
			})

			It("returns a status 422 with appropriate error message json", func() {
				expectUnprocessableEntityError(validatorErr, "metadata.labels.foo.cloudfoundry.org/bar label/annotation key cannot use the cloudfoundry.org domain")
			})
		})
	})

	Describe("SpacePatch", func() {
		var (
			payload        payloads.SpacePatch
			decodedPayload *payloads.SpacePatch
			validatorErr   error
		)

		BeforeEach(func() {
			payload = payloads.SpacePatch{
				Metadata: payloads.MetadataPatch{
					Annotations: map[string]*string{
						"foo": tools.PtrTo("bar"),
					},
					Labels: map[string]*string{
						"bob": tools.PtrTo("alice"),
					},
				},
			}

			decodedPayload = new(payloads.SpacePatch)
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

	Describe("SpaceList", func() {
		Describe("decoding from url values", func() {
			It("gets the names and organization_guids param and allows order_by", func() {
				spaceList := payloads.SpaceList{}
				req, err := http.NewRequest("GET", "http://foo.com/bar?names=foo,bar&organization_guids=o1,o2&order_by=name", nil)
				Expect(err).NotTo(HaveOccurred())
				err = validator.DecodeAndValidateURLValues(req, &spaceList)

				Expect(err).NotTo(HaveOccurred())
				Expect(spaceList.Names).To(Equal("foo,bar"))
				Expect(spaceList.OrganizationGUIDs).To(Equal("o1,o2"))
			})
		})

		Describe("ToMessage", func() {
			It("splits names to strings", func() {
				spaceList := payloads.SpaceList{
					Names:             "foo,bar",
					OrganizationGUIDs: "org1,org2",
				}
				Expect(spaceList.ToMessage().Names).To(ConsistOf("foo", "bar"))
				Expect(spaceList.ToMessage().OrganizationGUIDs).To(ConsistOf("org1", "org2"))
			})
		})
	})
})
