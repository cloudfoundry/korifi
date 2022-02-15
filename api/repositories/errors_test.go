package repositories_test

import (
	"errors"

	. "code.cloudfoundry.org/cf-k8s-controllers/api/repositories"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Errors", func() {
	Describe("NotFoundError", func() {
		var e NotFoundError

		Describe("Error function", func() {
			It("with empty struct, has canned response", func() {
				e = NotFoundError{}
				Expect(e.Error()).To(Equal("not found"))
			})

			It("with ResourceType, prepends resource info", func() {
				e = NewNotFoundError("Foo Resource", nil)
				Expect(e.Error()).To(Equal("Foo Resource not found"))
			})

			It("with wrapped error, appends error into", func() {
				e = NewNotFoundError("", errors.New("wrapped error"))
				Expect(e.Error()).To(Equal("not found: wrapped error"))
			})

			It("with ResourceType and wrapped error, prepends resource and appends error info", func() {
				e = NewNotFoundError("Bar Resource", errors.New("wrapped error"))
				Expect(e.Error()).To(Equal("Bar Resource not found: wrapped error"))
			})
		})

		Describe("unwrap", func() {
			It("returns the embedded error", func() {
				embeddedErr := errors.New("boo!")
				e = NewNotFoundError("Foo", embeddedErr)
				Expect(e.Unwrap()).To(Equal(embeddedErr))
			})
		})
	})

	Describe("forbidden error", func() {
		It("with ResourceType and wrapped error, prepends resource and appends error info", func() {
			e := NewForbiddenError("Bar Resource", errors.New("wrapped error"))
			Expect(e.Error()).To(Equal("Bar Resource forbidden: wrapped error"))
		})
	})
})
