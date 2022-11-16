package payloads_test

import (
	"bytes"
	"encoding/json"
	"net/http"

	"code.cloudfoundry.org/korifi/api/apierrors"
	"code.cloudfoundry.org/korifi/api/handlers"
	"code.cloudfoundry.org/korifi/api/payloads"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gstruct"
)

var _ = Describe("PackageCreate", func() {
	var (
		createPayload payloads.PackageCreate
		packageCreate *payloads.PackageCreate
		validator     *handlers.DecoderValidator
		validatorErr  error
	)

	BeforeEach(func() {
		var err error
		validator, err = handlers.NewDefaultDecoderValidator()
		Expect(err).NotTo(HaveOccurred())

		packageCreate = new(payloads.PackageCreate)
		createPayload = payloads.PackageCreate{
			Type: "bits",
			Relationships: &payloads.PackageRelationships{
				App: &payloads.Relationship{
					Data: &payloads.RelationshipData{
						GUID: "some-guid",
					},
				},
			},
		}
	})

	JustBeforeEach(func() {
		body, err := json.Marshal(createPayload)
		Expect(err).NotTo(HaveOccurred())

		req, err := http.NewRequest("", "", bytes.NewReader(body))
		Expect(err).NotTo(HaveOccurred())

		validatorErr = validator.DecodeAndValidateJSONPayload(req, packageCreate)
	})

	It("succeeds", func() {
		Expect(validatorErr).NotTo(HaveOccurred())
		Expect(packageCreate).To(gstruct.PointTo(Equal(createPayload)))
	})

	When("type is empty", func() {
		BeforeEach(func() {
			createPayload.Type = ""
		})

		It("returns an appropriate error", func() {
			expectUnprocessableEntityError(validatorErr, "Type is a required field")
		})
	})

	When("type is not in the allowed list", func() {
		BeforeEach(func() {
			createPayload.Type = "foo"
		})

		It("returns an appropriate error", func() {
			expectUnprocessableEntityError(validatorErr, "Type must be one of ['bits']")
		})
	})

	When("relationships is not set", func() {
		BeforeEach(func() {
			createPayload.Relationships = nil
		})

		It("returns an appropriate error", func() {
			expectUnprocessableEntityError(validatorErr, "Relationships is a required field")
		})
	})

	When("relationships.app is not set", func() {
		BeforeEach(func() {
			createPayload.Relationships.App = nil
		})

		It("returns an appropriate error", func() {
			expectUnprocessableEntityError(validatorErr, "App is a required field")
		})
	})

	When("relationships.app.data is not set", func() {
		BeforeEach(func() {
			createPayload.Relationships.App.Data = nil
		})

		It("returns an appropriate error", func() {
			expectUnprocessableEntityError(validatorErr, "Data is a required field")
		})
	})

	When("relationships.app.data.guid is not set", func() {
		BeforeEach(func() {
			createPayload.Relationships.App.Data.GUID = ""
		})

		It("returns an appropriate error", func() {
			expectUnprocessableEntityError(validatorErr, "GUID is a required field")
		})
	})
})

func expectUnprocessableEntityError(err error, detail string) {
	Expect(err).To(HaveOccurred())
	Expect(err).To(BeAssignableToTypeOf(apierrors.UnprocessableEntityError{}))
	Expect(err.(apierrors.UnprocessableEntityError).Detail()).To(ContainSubstring(detail))
}
