package apierrors_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"

	"code.cloudfoundry.org/korifi/api/apierrors"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var _ = Describe("FromK8sError", func() {
	var (
		err       error
		actualErr error
	)

	BeforeEach(func() {
		err = nil
	})

	JustBeforeEach(func() {
		actualErr = apierrors.FromK8sError(err, "foo")
	})

	It("returns nil", func() {
		Expect(actualErr).To(BeNil())
	})

	When("unauthorized k8s error", func() {
		BeforeEach(func() {
			err = k8serrors.NewUnauthorized("bar")
		})

		It("translates it to invalid auth api error", func() {
			Expect(actualErr).To(Equal(apierrors.NewInvalidAuthError(err)))
		})
	})

	When("forbidden k8s error", func() {
		BeforeEach(func() {
			err = k8serrors.NewForbidden(schema.GroupResource{}, "blob", nil)
		})

		It("translates it to forbidden api error", func() {
			Expect(actualErr).To(Equal(apierrors.NewForbiddenError(err, "foo")))
		})
	})

	When("not found k8s error", func() {
		BeforeEach(func() {
			err = k8serrors.NewNotFound(schema.GroupResource{}, "jim")
		})

		It("translates it to not found api error", func() {
			Expect(actualErr).To(Equal(apierrors.NewNotFoundError(err, "foo")))
		})
	})

	When("unknown error", func() {
		BeforeEach(func() {
			err = errors.New("bar")
		})

		It("returns it", func() {
			Expect(actualErr).To(Equal(err))
		})
	})
})

var _ = Describe("ForbiddenAsNotFound", func() {
	var (
		err       error
		actualErr error
	)

	BeforeEach(func() {
		err = nil
	})

	JustBeforeEach(func() {
		actualErr = apierrors.ForbiddenAsNotFound(err)
	})

	It("returns nil", func() {
		Expect(actualErr).To(BeNil())
	})

	When("forbidden error", func() {
		BeforeEach(func() {
			err = apierrors.NewForbiddenError(errors.New("foo"), "bar")
		})

		It("returns a not found error", func() {
			var notFoundError apierrors.NotFoundError
			Expect(errors.As(actualErr, &notFoundError)).To(BeTrue())
			Expect(notFoundError.Detail()).To(Equal("bar not found. Ensure it exists and you have access to it."))
			Expect(notFoundError.Unwrap()).To(MatchError("foo"))
		})
	})

	When("any other error", func() {
		BeforeEach(func() {
			err = errors.New("other")
		})

		It("returns it", func() {
			Expect(actualErr).To(Equal(err))
		})
	})
})

var _ = Describe("DropletForbiddenAsNotFound", func() {
	var (
		err       error
		actualErr error
	)

	BeforeEach(func() {
		err = nil
	})

	JustBeforeEach(func() {
		actualErr = apierrors.DropletForbiddenAsNotFound(err)
	})

	It("returns nil", func() {
		Expect(actualErr).To(BeNil())
	})

	When("forbidden error", func() {
		BeforeEach(func() {
			err = apierrors.NewForbiddenError(errors.New("foo"), "Droplet")
		})

		It("returns a not found error", func() {
			var notFoundError apierrors.NotFoundError
			Expect(errors.As(actualErr, &notFoundError)).To(BeTrue())
			Expect(notFoundError.Detail()).To(Equal("Droplet not found"))
			Expect(notFoundError.Unwrap()).To(MatchError("foo"))
		})
	})

	When("not found error", func() {
		BeforeEach(func() {
			err = apierrors.NewNotFoundError(errors.New("foo"), "Droplet")
		})

		It("returns a not found error", func() {
			var notFoundError apierrors.NotFoundError
			Expect(errors.As(actualErr, &notFoundError)).To(BeTrue())
			Expect(notFoundError.Detail()).To(Equal("Droplet not found"))
			Expect(notFoundError.Unwrap()).To(MatchError("foo"))
		})
	})

	When("any other error", func() {
		BeforeEach(func() {
			err = errors.New("other")
		})

		It("returns it", func() {
			Expect(actualErr).To(Equal(err))
		})
	})
})

var _ = Describe("AsUnprocessibleEntity", func() {
	var (
		err       error
		actualErr error
	)

	BeforeEach(func() {
		err = nil
	})

	JustBeforeEach(func() {
		actualErr = apierrors.AsUnprocessableEntity(err, "detail", apierrors.ForbiddenError{}, apierrors.NotFoundError{})
	})

	It("returns nil", func() {
		Expect(actualErr).To(BeNil())
	})

	When("forbidden error", func() {
		BeforeEach(func() {
			err = apierrors.NewForbiddenError(errors.New("foo"), "bar")
		})

		It("returns an unprocessable entity error", func() {
			var unprocessableEntityError apierrors.UnprocessableEntityError
			Expect(errors.As(actualErr, &unprocessableEntityError)).To(BeTrue())
			Expect(unprocessableEntityError.Detail()).To(Equal("detail"))
			Expect(unprocessableEntityError.Unwrap()).To(MatchError("foo"))
		})
	})

	When("not found error", func() {
		BeforeEach(func() {
			err = apierrors.NewNotFoundError(errors.New("foo"), "bar")
		})

		It("returns an unprocessable entity error", func() {
			var unprocessableEntityError apierrors.UnprocessableEntityError
			Expect(errors.As(actualErr, &unprocessableEntityError)).To(BeTrue())
			Expect(unprocessableEntityError.Detail()).To(Equal("detail"))
			Expect(unprocessableEntityError.Unwrap()).To(MatchError("foo"))
		})
	})

	When("an error assignable to ApiError but different from translated ones", func() {
		BeforeEach(func() {
			err = testApiError{}
		})

		It("returns the error as is", func() {
			Expect(actualErr).To(Equal(err))
		})
	})

	When("wrapped error", func() {
		BeforeEach(func() {
			err = fmt.Errorf("foo: %w", apierrors.NewForbiddenError(errors.New("foo"), "bar"))
		})

		It("returns an unprocessable entity error", func() {
			var unprocessableEntityError apierrors.UnprocessableEntityError
			Expect(errors.As(actualErr, &unprocessableEntityError)).To(BeTrue())
			Expect(unprocessableEntityError.Detail()).To(Equal("detail"))
			Expect(unprocessableEntityError.Unwrap()).To(MatchError("foo"))
		})
	})

	When("any other error", func() {
		BeforeEach(func() {
			err = errors.New("other")
		})

		It("returns it", func() {
			Expect(actualErr).To(Equal(err))
		})
	})
})

var _ = Describe("LogAndReturn", func() {
	var (
		originalErr error
		logBuf      *bytes.Buffer
		logEntry    map[string]interface{}
	)

	BeforeEach(func() {
		logBuf = new(bytes.Buffer)
	})

	JustBeforeEach(func() {
		logger := zap.New(zap.WriteTo(logBuf))
		returnedErr := apierrors.LogAndReturn(logger, originalErr, "some message", "some-key", "some-value")
		Expect(returnedErr).To(Equal(originalErr))
		Expect(json.Unmarshal(logBuf.Bytes(), &logEntry)).To(Succeed())
	})

	When("the erorr is not an ApiError", func() {
		BeforeEach(func() {
			originalErr = errors.New("not-api-error")
		})

		It("logs an error", func() {
			Expect(logEntry["level"]).To(Equal("error"))
			Expect(logEntry["msg"]).To(Equal("some message"))
			Expect(logEntry["some-key"]).To(Equal("some-value"))
			Expect(logEntry["error"]).To(Equal("not-api-error"))
		})
	})

	When("the error is an ApiError", func() {
		BeforeEach(func() {
			originalErr = apierrors.NewForbiddenError(errors.New("cause-err"), "my-resource")
		})

		It("logs an info", func() {
			Expect(logEntry["level"]).To(Equal("info"))
			Expect(logEntry["msg"]).To(Equal("some message"))
			Expect(logEntry["some-key"]).To(Equal("some-value"))
			Expect(logEntry["err"]).To(Equal("cause-err"))
		})
	})

	When("the error is a wrapped ApiError", func() {
		BeforeEach(func() {
			originalErr = fmt.Errorf("wrapping: %w", apierrors.NewForbiddenError(errors.New("cause-err"), "my-resource"))
		})

		It("logs an info", func() {
			Expect(logEntry["level"]).To(Equal("info"))
			Expect(logEntry["msg"]).To(Equal("some message"))
			Expect(logEntry["some-key"]).To(Equal("some-value"))
			Expect(logEntry["err"]).To(Equal("wrapping: cause-err"))
		})
	})
})

type testApiError struct {
	apierrors.ApiError
}

func (e testApiError) Error() string {
	return ""
}

func (e testApiError) Unwrap() error {
	return nil
}
