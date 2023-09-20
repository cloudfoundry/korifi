package payloads_test

import (
	"code.cloudfoundry.org/korifi/api/payloads"
	"code.cloudfoundry.org/korifi/api/repositories"
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
			validatorErr = validator.DecodeAndValidateJSONPayload(createJSONRequest(payload), decodedPayload)
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
	})

	Describe("SpaceList", func() {
		Describe("Validation", func() {
			DescribeTable("valid query",
				func(query string, expectedSpaceList payloads.SpaceList) {
					actualSpaceList, decodeErr := decodeQuery[payloads.SpaceList](query)

					Expect(decodeErr).NotTo(HaveOccurred())
					Expect(*actualSpaceList).To(Equal(expectedSpaceList))
				},

				Entry("names", "names=name", payloads.SpaceList{Names: "name"}),
				Entry("guids", "guids=guid", payloads.SpaceList{GUIDs: "guid"}),
				Entry("organization_guids", "organization_guids=org-guid", payloads.SpaceList{OrganizationGUIDs: "org-guid"}),
				Entry("order_by", "order_by=something", payloads.SpaceList{}),
				Entry("per_page", "per_page=few", payloads.SpaceList{}),
				Entry("page", "page=3", payloads.SpaceList{}),
			)
		})

		Describe("ToMessage", func() {
			It("splits names to strings", func() {
				spaceList := payloads.SpaceList{
					Names:             "foo,bar",
					GUIDs:             "g1,g2",
					OrganizationGUIDs: "org1,org2",
				}
				Expect(spaceList.ToMessage()).To(Equal(repositories.ListSpacesMessage{
					Names:             []string{"foo", "bar"},
					GUIDs:             []string{"g1", "g2"},
					OrganizationGUIDs: []string{"org1", "org2"},
				}))
			})
		})
	})
})
