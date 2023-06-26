package payloads_test

import (
	"code.cloudfoundry.org/korifi/api/payloads"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gstruct"
)

var _ = Describe("Relationship", func() {
	var (
		relationshipPayload        payloads.Relationship
		decodedRelationshipPayload *payloads.Relationship
		validatorErr               error
	)

	BeforeEach(func() {
		decodedRelationshipPayload = new(payloads.Relationship)
		relationshipPayload = payloads.Relationship{
			Data: &payloads.RelationshipData{
				GUID: "the-guid",
			},
		}
	})

	JustBeforeEach(func() {
		validatorErr = validator.DecodeAndValidateJSONPayload(createRequest(relationshipPayload), decodedRelationshipPayload)
	})

	It("succeeds", func() {
		Expect(validatorErr).NotTo(HaveOccurred())
		Expect(decodedRelationshipPayload).To(gstruct.PointTo(Equal(relationshipPayload)))
	})

	When("data is empty", func() {
		BeforeEach(func() {
			relationshipPayload.Data = nil
		})

		It("returns an appropriate error", func() {
			expectUnprocessableEntityError(validatorErr, "data is required")
		})
	})

	When("data has empty guid", func() {
		BeforeEach(func() {
			relationshipPayload.Data.GUID = ""
		})

		It("returns an appropriate error", func() {
			expectUnprocessableEntityError(validatorErr, "guid cannot be blank")
		})
	})
})
