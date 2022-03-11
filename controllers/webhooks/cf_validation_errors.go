package webhooks

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
	DuplicateAppError
	DuplicateOrgNameError
	DuplicateSpaceNameError
	DuplicateRouteError
	DuplicateDomainError
	DuplicateServiceInstanceNameError
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
	case DuplicateAppError:
		return "CFApp with the same spec.name exists"
	case DuplicateOrgNameError:
		return "Org with same name exists"
	case DuplicateSpaceNameError:
		return "Space with same name exists"
	case DuplicateRouteError:
		return "Route with same FQDN and path exists"
	case DuplicateDomainError:
		return "Overlapping domain exists"
	case DuplicateServiceInstanceNameError:
		return "CFServiceInstance with same spec.name exists"
	default:
		return "An unknown error has occurred"
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
