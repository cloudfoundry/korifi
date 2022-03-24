package webhooks_test

import (
	"fmt"

	"code.cloudfoundry.org/cf-k8s-controllers/controllers/webhooks"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("CFWebhookValidationError", func() {
	It("Marshals a payload", func() {
		e := webhooks.DuplicateAppError
		Expect(e.Marshal()).To(Equal(`{"code":1,"message":"CFApp with the same spec.name exists"}`))
	})

	It("Unmarshals UnknownError", func() {
		p := `{"code":0}`
		Expect(webhooks.ExtractCodeFromErrorReason(p)).To(Equal(webhooks.UnknownError))
	})

	It("Unmarshals DuplicateAppError", func() {
		p := `{"code":1}`
		Expect(webhooks.ExtractCodeFromErrorReason(p)).To(Equal(webhooks.DuplicateAppError))
	})

	It("Handles malformed json payloads", func() {
		p := `{"code":1`
		Expect(webhooks.ExtractCodeFromErrorReason(p)).To(Equal(webhooks.UnknownError))
	})
})

var _ = Describe("HasErrorCode", func() {
	var err error

	BeforeEach(func() {
		err = &errors.StatusError{
			ErrStatus: metav1.Status{
				Reason:  metav1.StatusReason(fmt.Sprintf(`{"code":%d,"message":"foo"}`, webhooks.DuplicateAppError)),
				Message: "oops",
			},
		}
	})

	When("error matches the expected error code", func() {
		It("returns true", func() {
			Expect(webhooks.HasErrorCode(err, webhooks.DuplicateAppError)).To(BeTrue())
		})
	})

	When("error is a different webhooks error", func() {
		It("returns false", func() {
			Expect(webhooks.HasErrorCode(err, webhooks.UnknownError)).To(BeFalse())
		})
	})

	When("error is not a k8s status error", func() {
		It("returns false", func() {
			err = fmt.Errorf("foo")
			Expect(webhooks.HasErrorCode(err, webhooks.DuplicateAppError)).To(BeFalse())
		})
	})
})
