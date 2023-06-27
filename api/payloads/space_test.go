package payloads_test

import (
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
})
