package errors_test

import (
	"errors"

	desc_errs "code.cloudfoundry.org/korifi/api/repositories/k8sklient/descriptors/errors"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var _ = Describe("IsObjectResolutionError", func() {
	var (
		isObjectResolutionErr bool
		err                   error
	)

	BeforeEach(func() {
		err = nil
	})

	JustBeforeEach(func() {
		isObjectResolutionErr = desc_errs.IsObjectResolutionError(err)
	})

	It("returns false", func() {
		Expect(isObjectResolutionErr).To(BeFalse())
	})

	When("the error is a ObjectResulutionError", func() {
		BeforeEach(func() {
			err = desc_errs.NewObjectResolutionError("1234", schema.GroupVersionKind{})
		})

		It("returns true", func() {
			Expect(isObjectResolutionErr).To(BeTrue())
		})
	})

	When("the error is a random error", func() {
		BeforeEach(func() {
			err = errors.New("foo")
		})

		It("returns false", func() {
			Expect(isObjectResolutionErr).To(BeFalse())
		})
	})
})
