package workloads_test

import (
	"fmt"

	"code.cloudfoundry.org/cf-k8s-controllers/webhooks/workloads"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("CFWorkloadsWebhookValidationError", func() {
	It("Marshals a payload", func() {
		e := workloads.DuplicateAppError
		Expect(e.Marshal()).To(Equal(`{"code":1,"message":"CFApp with the same spec.name exists"}`))
	})

	It("Unmarshals UnknownError", func() {
		e := new(workloads.ValidationErrorCode)
		p := `{"code":0}`
		e.Unmarshall(p)
		Expect(*e).To(Equal(workloads.UnknownError))
	})

	It("Unmarshals DuplicateAppError", func() {
		e := new(workloads.ValidationErrorCode)
		p := `{"code":1}`
		e.Unmarshall(p)
		Expect(*e).To(Equal(workloads.DuplicateAppError))
	})

	It("Handles malformed json payloads", func() {
		e := new(workloads.ValidationErrorCode)
		p := `{"code":1`
		e.Unmarshall(p)
		Expect(*e).To(Equal(workloads.UnknownError))
	})
})

var _ = Describe("HasErrorCode", func() {
	var err error

	BeforeEach(func() {
		err = &errors.StatusError{
			ErrStatus: metav1.Status{
				Reason:  metav1.StatusReason(fmt.Sprintf(`{"code":%d,"message":"foo"}`, workloads.DuplicateAppError)),
				Message: "oops",
			},
		}
	})

	When("error matches the expected error code", func() {
		It("returns true", func() {
			Expect(workloads.HasErrorCode(err, workloads.DuplicateAppError)).To(BeTrue())
		})
	})

	When("error is a different workloads error", func() {
		It("returns false", func() {
			Expect(workloads.HasErrorCode(err, workloads.UnknownError)).To(BeFalse())
		})
	})

	When("error is not a k8s status error", func() {
		It("returns false", func() {
			err = fmt.Errorf("foo")
			Expect(workloads.HasErrorCode(err, workloads.DuplicateAppError)).To(BeFalse())
		})
	})
})
