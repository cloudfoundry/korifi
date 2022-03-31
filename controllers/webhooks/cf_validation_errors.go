package webhooks

import (
	"encoding/json"
	"errors"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
)

const (
	UnknownErrorType    = "UnknownError"
	UnknownErrorMessage = "An unknown error has occurred"
)

type ValidationError struct {
	Type    string `json:"validationErrorType"`
	Message string `json:"message"`
}

func (v ValidationError) Error() string {
	return "ValidationError-" + v.Type + ": " + v.Message
}

func (v ValidationError) Marshal() string {
	bytes, err := json.Marshal(v)
	if err != nil { // This (probably) can't fail, untested
		return err.Error()
	}
	return string(bytes)
}

func WebhookErrorToValidationError(err error) (ValidationError, bool) {
	statusError := new(k8serrors.StatusError)
	if !errors.As(err, &statusError) {
		return ValidationError{}, false
	}

	validationError := new(ValidationError)
	if json.Unmarshal([]byte(statusError.Status().Reason), validationError) != nil {
		return ValidationError{}, false
	}
	return *validationError, true
}

func AdmissionUnknownErrorReason() string {
	return ValidationError{Type: UnknownErrorType, Message: UnknownErrorMessage}.Marshal()
}
