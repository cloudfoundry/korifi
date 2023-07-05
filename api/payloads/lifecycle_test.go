package payloads_test

import (
	"code.cloudfoundry.org/korifi/api/payloads"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gstruct"
)

var _ = Describe("Lifecycle", func() {
	var (
		payload        payloads.Lifecycle
		decodedPayload *payloads.Lifecycle
		validatorErr   error
	)

	BeforeEach(func() {
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
		validatorErr = validator.DecodeAndValidateJSONPayload(createJSONRequest(payload), decodedPayload)
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

var _ = Describe("LifecyclePatch", func() {
	var (
		payload        payloads.LifecyclePatch
		decodedPayload *payloads.LifecyclePatch
		validatorErr   error
	)

	BeforeEach(func() {
		payload = payloads.LifecyclePatch{
			Type: "buildpack",
			Data: &payloads.LifecycleDataPatch{
				Buildpacks: &[]string{"foo", "bar"},
			},
		}

		decodedPayload = new(payloads.LifecyclePatch)
	})

	JustBeforeEach(func() {
		validatorErr = validator.DecodeAndValidateJSONPayload(createJSONRequest(payload), decodedPayload)
	})

	It("succeeds", func() {
		Expect(validatorErr).NotTo(HaveOccurred())
		Expect(decodedPayload).To(gstruct.PointTo(Equal(payload)))
	})

	When("lifecycle data is not set", func() {
		BeforeEach(func() {
			payload.Data = nil
		})

		It("returns an appropriate error", func() {
			expectUnprocessableEntityError(validatorErr, "data is required")
		})
	})

	When("lifecycle.type is not buildpack", func() {
		BeforeEach(func() {
			payload.Type = "not-buildpack"
		})

		It("returns an error", func() {
			expectUnprocessableEntityError(validatorErr, "type value must be one of: buildpack")
		})
	})

	When("lifecycle.type is empty", func() {
		BeforeEach(func() {
			payload.Type = ""
		})

		It("does not default it", func() {
			Expect(validatorErr).NotTo(HaveOccurred())
			Expect(decodedPayload.Type).To(BeEmpty())
		})
	})
})
