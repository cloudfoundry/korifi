package payloads_test

import (
	"code.cloudfoundry.org/korifi/api/handlers"
	"code.cloudfoundry.org/korifi/api/payloads"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gstruct"
)

var _ = Describe("Lifecycle", func() {
	var (
		decoderValidator handlers.DecoderValidator
		payload          payloads.Lifecycle
		decodedPayload   *payloads.Lifecycle
		validatorErr     error
	)

	BeforeEach(func() {
		decoderValidator = handlers.NewDefaultDecoderValidator()

		payload = payloads.Lifecycle{
			Type: "buildpack",
			Data: &payloads.LifecycleData{
				Buildpacks: []string{"foo", "bar"},
				Stack:      "baz",
			},
		}

		decodedPayload = new(payloads.Lifecycle)
	})

	JustBeforeEach(func() {
		validatorErr = decoderValidator.DecodeAndValidateJSONPayload(createRequest(payload), decodedPayload)
	})

	It("succeeds", func() {
		Expect(validatorErr).NotTo(HaveOccurred())
		Expect(decodedPayload).To(gstruct.PointTo(Equal(payload)))
	})

	When("type is not set", func() {
		BeforeEach(func() {
			payload.Type = ""
		})

		It("returns an appropriate error", func() {
			expectUnprocessableEntityError(validatorErr, "type cannot be blank")
		})
	})

	When("lifecycle data is not set", func() {
		BeforeEach(func() {
			payload.Data = nil
		})

		It("returns an appropriate error", func() {
			expectUnprocessableEntityError(validatorErr, "data is required")
		})
	})

	When("stack is not set", func() {
		BeforeEach(func() {
			payload.Data.Stack = ""
		})

		It("returns an appropriate error", func() {
			expectUnprocessableEntityError(validatorErr, "data.stack cannot be blank")
		})
	})
})
