package repositories_test

import (
	"errors"

	"code.cloudfoundry.org/cf-k8s-controllers/api/repositories"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Errors", func() {
	DescribeTable("Not found errors",
		func(resourceType string, wrappedErr error, expectedMessage string) {
			notFoundError := repositories.NewNotFoundError(resourceType, wrappedErr)
			Expect(notFoundError.Error()).To(Equal(expectedMessage))
		},

		Entry("with ResourceType, prepends resource info",
			"Foo Resource", nil, "Foo Resource not found"),
		Entry("with wrapped error, appends error info",
			"", errors.New("wrapped error"), "not found: wrapped error"),
		Entry("with ResourceType and wrapped error, prepends resource and appends error info",
			"Bar Resource", errors.New("wrapped error"), "Bar Resource not found: wrapped error"),
	)

	DescribeTable("Forbidden errors",
		func(resourceType string, wrappedErr error, expectedMessage string) {
			forbiddenError := repositories.NewForbiddenError(resourceType, wrappedErr)
			Expect(forbiddenError.Error()).To(Equal(expectedMessage))
		},

		Entry("with ResourceType, prepends resource info",
			"Foo Resource", nil, "Foo Resource forbidden"),
		Entry("with wrapped error, appends error info",
			"", errors.New("wrapped error"), "forbidden: wrapped error"),
		Entry("with ResourceType and wrapped error, prepends resource and appends error info",
			"Bar Resource", errors.New("wrapped error"), "Bar Resource forbidden: wrapped error"),
	)

	Describe("unwrapping", func() {
		It("returns the embedded error from not found error", func() {
			Expect(repositories.NewNotFoundError("Foo", errors.New("boo")).Unwrap()).To(MatchError("boo"))
		})

		It("returns the embedded error from forbidden error", func() {
			Expect(repositories.NewForbiddenError("Foo", errors.New("boo")).Unwrap()).To(MatchError("boo"))
		})
	})
})
