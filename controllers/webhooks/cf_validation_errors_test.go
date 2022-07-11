package webhooks_test

import (
	"errors"

	. "code.cloudfoundry.org/korifi/controllers/webhooks"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("CFWebhookValidationError", func() {
	var (
		validationErrorType, validationErrorMessage string
		validationErr                               ValidationError
	)
	BeforeEach(func() {
		validationErrorType = "some-validation-error-type"
		validationErrorMessage = "some validation error message"
		validationErr = ValidationError{
			Type:    validationErrorType,
			Message: validationErrorMessage,
		}
	})

	Describe("Error", func() {
		It("returns a formatted error string", func() {
			Expect(validationErr.Error()).To(Equal("ValidationError-" + validationErr.Type + ": " + validationErr.Message))
		})
	})

	Describe("GetMessage", func() {
		It("returns the user-facing message", func() {
			Expect(validationErr.GetMessage()).To(Equal("some validation error message"))
		})
	})

	Describe("ExportJSONError", func() {
		It("returns an error with the JSON representation of the validation error", func() {
			expectedBody := `{"validationErrorType":"` + validationErrorType + `","message":"` + validationErrorMessage + `"}`
			Expect(validationErr.ExportJSONError()).To(MatchError(MatchJSON(expectedBody)))
		})
	})
})

var _ = Describe("WebhookErrorToValidationError", func() {
	var (
		validationErrorType, validationErrorMessage string
		inputErr                                    error
		validationErr                               ValidationError
		isValidationError                           bool
	)

	BeforeEach(func() {
		validationErrorType = "some-validation-error-type"
		validationErrorMessage = "some validation error message"
		inputErr = &k8serrors.StatusError{
			ErrStatus: metav1.Status{
				Reason:  metav1.StatusReason(`{"validationErrorType":"` + validationErrorType + `","message":"` + validationErrorMessage + `"}`),
				Message: "oops",
			},
		}
	})

	JustBeforeEach(func() {
		validationErr, isValidationError = WebhookErrorToValidationError(inputErr)
	})

	It("unmarshals a K8s-wrapped validation error into a ValidationError, and returns true", func() {
		Expect(isValidationError).To(BeTrue())
		Expect(validationErr).To(Equal(ValidationError{
			Type:    validationErrorType,
			Message: validationErrorMessage,
		}))
	})

	When("the error is not a K8s error", func() {
		BeforeEach(func() {
			inputErr = errors.New("some-random-error")
		})

		It("returns an empty ValidationError and false", func() {
			Expect(isValidationError).To(BeFalse())
			Expect(validationErr).To(Equal(ValidationError{}))
		})
	})
})
