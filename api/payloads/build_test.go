package payloads_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gstruct"

	"code.cloudfoundry.org/korifi/api/payloads"
)

var _ = Describe("BuildCreate", func() {
	Describe("Decode", func() {
		var (
			createPayload       payloads.BuildCreate
			decodedBuildPayload *payloads.BuildCreate
			validatorErr        error
		)

		BeforeEach(func() {
			decodedBuildPayload = new(payloads.BuildCreate)
			createPayload = payloads.BuildCreate{
				Package: &payloads.RelationshipData{
					GUID: "some-build-guid",
				},
			}
		})

		JustBeforeEach(func() {
			validatorErr = validator.DecodeAndValidateJSONPayload(createRequest(createPayload), decodedBuildPayload)
		})

		It("succeeds", func() {
			Expect(validatorErr).NotTo(HaveOccurred())
			Expect(decodedBuildPayload).To(gstruct.PointTo(Equal(createPayload)))
		})

		When("package is not provided", func() {
			BeforeEach(func() {
				createPayload.Package = nil
			})

			It("says package is required", func() {
				expectUnprocessableEntityError(validatorErr, "Package is a required field")
			})
		})

		When("package guid is empty", func() {
			BeforeEach(func() {
				createPayload.Package.GUID = ""
			})

			It("says guid is required", func() {
				expectUnprocessableEntityError(validatorErr, "GUID is a required field")
			})
		})

		When("the metadata annotations is not empty", func() {
			BeforeEach(func() {
				createPayload.Metadata.Annotations = map[string]string{
					"foo": "bar",
				}
			})

			It("says labels and annotations are not supported", func() {
				expectUnprocessableEntityError(validatorErr, "Labels and annotations are not supported for builds")
			})
		})

		When("the metadata labels is not empty", func() {
			BeforeEach(func() {
				createPayload.Metadata.Labels = map[string]string{
					"foo": "bar",
				}
			})

			It("says labels and annotations are not supported", func() {
				expectUnprocessableEntityError(validatorErr, "Labels and annotations are not supported for builds")
			})
		})
	})
})
