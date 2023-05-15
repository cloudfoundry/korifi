package payloads_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"code.cloudfoundry.org/korifi/api/payloads"
)

var _ = Describe("BuildCreate", func() {
	Describe("Decode", func() {
		var (
			createPayload *payloads.BuildCreate
			validatorErr  error
		)

		BeforeEach(func() {
			createPayload = &payloads.BuildCreate{
				Package: &payloads.RelationshipData{
					GUID: "some-build-guid",
				},
			}
		})

		JustBeforeEach(func() {
			validatorErr = validator.ValidatePayload(createPayload)
		})

		It("succeeds", func() {
			Expect(validatorErr).NotTo(HaveOccurred())
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
