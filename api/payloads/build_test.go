package payloads_test

import (
	"bytes"
	"code.cloudfoundry.org/korifi/tools"
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

var _ = Describe("BuildUpdate", func() {
	Describe("Decode", func() {
		var (
			updatePayload       payloads.BuildUpdate
			decodedBuildPayload *payloads.BuildUpdate
			validatorErr        error
		)

		BeforeEach(func() {
			decodedBuildPayload = new(payloads.BuildUpdate)
			updatePayload = payloads.BuildUpdate{
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
			body, err := json.Marshal(updatePayload)
			Expect(err).NotTo(HaveOccurred())

			req, err := http.NewRequest("", "", bytes.NewReader(body))
			Expect(err).NotTo(HaveOccurred())

			validatorErr = validator.DecodeAndValidateJSONPayload(req, decodedBuildPayload)
		})

		It("succeeds", func() {
			Expect(validatorErr).NotTo(HaveOccurred())
			Expect(decodedBuildPayload).To(gstruct.PointTo(Equal(updatePayload)))
		})

		When("metadata.labels contains an invalid key", func() {
			BeforeEach(func() {
				updatePayload.Metadata = payloads.MetadataPatch{
					Labels: map[string]*string{
						"foo.cloudfoundry.org/bar": tools.PtrTo("jim"),
					},
				}
			})

			It("returns an appropriate error", func() {
				expectUnprocessableEntityError(validatorErr, "cannot begin with \"cloudfoundry.org\"")
			})
		})

		When("metadata.annotations contains an invalid key", func() {
			BeforeEach(func() {
				updatePayload.Metadata = payloads.MetadataPatch{
					Annotations: map[string]*string{
						"foo.cloudfoundry.org/bar": tools.PtrTo("jim"),
					},
				}
			})

			It("returns an appropriate error", func() {
				expectUnprocessableEntityError(validatorErr, "cannot begin with \"cloudfoundry.org\"")
			})
		})
	})
})
