package networking

import (
	"encoding/json"
	"errors"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
)

type ValidationErrorCode int

type ValidationError struct {
	Code    ValidationErrorCode `json:"code"`
	Message string              `json:"message"`
}

const (
	UnknownError = ValidationErrorCode(iota)
	DuplicateRouteError
)

func (w ValidationErrorCode) Marshal() string {
	bytes, err := json.Marshal(ValidationError{
		Code:    w,
		Message: w.GetMessage(),
	})
	if err != nil {
		return err.Error()
	}
	return string(bytes)
}

func (w *ValidationErrorCode) Unmarshall(payload string) {
	validationErr := new(ValidationError)
	err := json.Unmarshal([]byte(payload), validationErr)
	if err != nil {
		*w = UnknownError
	}
	*w = validationErr.Code
}

func (w ValidationErrorCode) GetMessage() string {
	switch w {
	case DuplicateRouteError:
		return "CFRoute with the same spec.host exists for the CFDomain"
	default:
		return "An unknown error has occured"
	}
}

func HasErrorCode(err error, code ValidationErrorCode) bool {
	if statusError := new(k8serrors.StatusError); errors.As(err, &statusError) {
		reason := statusError.Status().Reason

		val := new(ValidationErrorCode)
		val.Unmarshall(string(reason))

		if *val == code {
			return true
		}
	}
	return false
}
