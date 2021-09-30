package workloads_test

import (
	. "code.cloudfoundry.org/cf-k8s-controllers/webhooks/workloads"
	. "github.com/onsi/ginkgo"

	. "github.com/onsi/gomega"
)

var _ = Describe("CFWorkloadsWebhookValidationError Unit Tests", func() {
	It("Marshals a payload", func() {
		e := DuplicateAppError
		Expect(e.Marshal()).To(Equal(`{"code":1,"message":"CFApp with the same spec.name exists"}`))
	})

	It("Unmarshals UnknownError", func() {
		e := new(ValidationErrorCode)
		p := `{"code":0}`
		e.Unmarshall(p)
		Expect(*e).To(Equal(UnknownError))
	})

	It("Unmarshals DuplicateAppError", func() {
		e := new(ValidationErrorCode)
		p := `{"code":1}`
		e.Unmarshall(p)
		Expect(*e).To(Equal(DuplicateAppError))
	})

	It("Handles malformed json payloads", func() {
		e := new(ValidationErrorCode)
		p := `{"code":1`
		e.Unmarshall(p)
		Expect(*e).To(Equal(UnknownError))
	})
})
