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
	RouteDestinationNotInSpace
	RouteFQDNInvalidError
	HostNameIsInvalidError
	PathValidationError
)

func (v ValidationError) Marshal() string {
	bytes, err := json.Marshal(v)
	if err != nil {
		return err.Error()
	}
	return string(bytes)
}

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
	case RouteDestinationNotInSpace:
		return "Route destination app not found in space"
	case RouteFQDNInvalidError:
		return "Route FQDN does not comply with RFC 1035 standards"
	case HostNameIsInvalidError:
		return "Missing or Invalid host - Routes in shared domains must have a valid host defined"
	default:
		return "An unknown error has occurred"
	}
}

func ExtractCodeFromErrorReason(payload string) ValidationErrorCode {
	validationErr := new(ValidationError)
	err := json.Unmarshal([]byte(payload), validationErr)
	if err != nil {
		return UnknownError
	}
	return validationErr.Code
}

func HasErrorCode(err error, hasCode ValidationErrorCode) bool {
	statusError := new(k8serrors.StatusError)
	if !errors.As(err, &statusError) {
		return false
	}
	errorCode := ExtractCodeFromErrorReason(string(statusError.Status().Reason))
	return errorCode == hasCode
}

func IsValidationError(err error) bool {
	if statusError := new(k8serrors.StatusError); errors.As(err, &statusError) {
		reason := statusError.Status().Reason

		errorCode := ExtractCodeFromErrorReason(string(reason))

		if errorCode != UnknownError {
			return true
		}
	}
	return false
}

func GetErrorMessage(err error) string {
	if statusError := new(k8serrors.StatusError); errors.As(err, &statusError) {
		reason := statusError.Status().Reason

		val := new(ValidationError)
		err := json.Unmarshal([]byte(reason), val)
		if err != nil {
			return ""
		}

		return val.Message
	}
	return ""
}
