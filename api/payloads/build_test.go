package payloads_test

import (
	"bytes"
	"encoding/json"
	"net/http"

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
			body, err := json.Marshal(createPayload)
			Expect(err).NotTo(HaveOccurred())

			req, err := http.NewRequest("", "", bytes.NewReader(body))
			Expect(err).NotTo(HaveOccurred())

			validatorErr = validator.DecodeAndValidateJSONPayload(req, decodedBuildPayload)
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
	})
})
