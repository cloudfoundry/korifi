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
				e = NotFoundError{ResourceType: "Foo Resource"}
				Expect(e.Error()).To(Equal("Foo Resource not found"))
			})

			It("with wrapped error, appends error into", func() {
				e = NotFoundError{Err: errors.New("wrapped error")}
				Expect(e.Error()).To(Equal("not found: wrapped error"))
			})

			It("with ResourceType and wrapped error, prepends resource and appends error info", func() {
				e = NotFoundError{ResourceType: "Bar Resource", Err: errors.New("wrapped error")}
				Expect(e.Error()).To(Equal("Bar Resource not found: wrapped error"))
			})
		})
	})
})
