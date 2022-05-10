package apierrors

import (
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"strings"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
)

type ApiError interface {
	Detail() string
	Title() string
	Code() int
	HttpStatus() int
	Unwrap() error
	Error() string
}

type apiError struct {
	cause      error
	detail     string
	title      string
	code       int
	httpStatus int
}

func (e apiError) Error() string {
	if e.cause == nil {
		return "unknown"
	}

	return e.cause.Error()
}

func (e apiError) Unwrap() error {
	return e.cause
}

func (e apiError) Detail() string {
	return e.detail
}

func (e apiError) Title() string {
	return e.title
}

func (e apiError) Code() int {
	return e.code
}

func (e apiError) HttpStatus() int {
	return e.httpStatus
}

type UnprocessableEntityError struct {
	apiError
}

func NewUnprocessableEntityError(cause error, detail string) UnprocessableEntityError {
	return UnprocessableEntityError{
		apiError{
			cause:      cause,
			title:      "CF-UnprocessableEntity",
			detail:     detail,
			code:       10008,
			httpStatus: http.StatusUnprocessableEntity,
		},
	}
}

type MessageParseError struct {
	apiError
}

func NewMessageParseError(cause error) MessageParseError {
	return MessageParseError{
		apiError{
			cause:      cause,
			title:      "CF-MessageParseError",
			detail:     "Request invalid due to parse error: invalid request body",
			code:       1001,
			httpStatus: http.StatusBadRequest,
		},
	}
}

type UnknownError struct {
	apiError
}

func NewUnknownError(cause error) UnknownError {
	return UnknownError{
		apiError{
			cause:      cause,
			title:      "UnknownError",
			detail:     "An unknown error occurred.",
			code:       10001,
			httpStatus: http.StatusInternalServerError,
		},
	}
}

type NotFoundError struct {
	apiError
}

func NewNotFoundError(cause error, resourceType string) NotFoundError {
	return NotFoundError{
		apiError{
			cause:      cause,
			title:      "CF-ResourceNotFound",
			detail:     fmt.Sprintf("%s not found. Ensure it exists and you have access to it.", resourceType),
			code:       10010,
			httpStatus: http.StatusNotFound,
		},
	}
}

type InvalidAuthError struct {
	apiError
}

func NewInvalidAuthError(cause error) InvalidAuthError {
	return InvalidAuthError{
		apiError{
			cause:      cause,
			title:      "CF-InvalidAuthToken",
			detail:     "Invalid Auth Token",
			code:       1000,
			httpStatus: http.StatusUnauthorized,
		},
	}
}

type NotAuthenticatedError struct {
	apiError
}

func NewNotAuthenticatedError(cause error) NotAuthenticatedError {
	return NotAuthenticatedError{
		apiError{
			cause:      cause,
			title:      "CF-NotAuthenticated",
			detail:     "Authentication error",
			code:       10002,
			httpStatus: http.StatusUnauthorized,
		},
	}
}

type ForbiddenError struct {
	apiError
	resourceType string
}

func (e ForbiddenError) ResourceType() string {
	return e.resourceType
}

func NewForbiddenError(cause error, resourceType string) ForbiddenError {
	return ForbiddenError{
		apiError: apiError{
			cause:      cause,
			title:      "CF-NotAuthorized",
			detail:     "You are not authorized to perform the requested action",
			code:       10003,
			httpStatus: http.StatusForbidden,
		},
		resourceType: resourceType,
	}
}

type UnknownKeyError struct {
	apiError
}

func NewUnknownKeyError(cause error, validKeys []string) UnknownKeyError {
	return UnknownKeyError{
		apiError: apiError{
			cause:      cause,
			title:      "CF-BadQueryParameter",
			detail:     fmt.Sprintf("The query parameter is invalid: Valid parameters are: '%s'", strings.Join(validKeys, ", ")),
			code:       10005,
			httpStatus: http.StatusBadRequest,
		},
	}
}

type UniquenessError struct {
	apiError
}

func NewUniquenessError(cause error, detail string) UniquenessError {
	return UniquenessError{
		apiError: apiError{
			cause:      cause,
			title:      "CF-UniquenessError",
			detail:     detail,
			code:       10016,
			httpStatus: http.StatusUnprocessableEntity,
		},
	}
}

type InvalidRequestError struct {
	apiError
}

func NewInvalidRequestError(cause error, detail string) InvalidRequestError {
	return InvalidRequestError{
		apiError: apiError{
			cause:      cause,
			title:      "CF-InvalidRequest",
			detail:     detail,
			code:       10004,
			httpStatus: http.StatusBadRequest,
		},
	}
}

type PackageBitsAlreadyUploadedError struct {
	apiError
}

func NewPackageBitsAlreadyUploadedError(cause error) PackageBitsAlreadyUploadedError {
	return PackageBitsAlreadyUploadedError{
		apiError: apiError{
			cause:      cause,
			title:      "CF-PackageBitsAlreadyUploaded",
			detail:     "Bits may be uploaded only once. Create a new package to upload different bits.",
			code:       150004,
			httpStatus: http.StatusBadRequest,
		},
	}
}

func FromK8sError(err error, resourceType string) error {
	switch {
	case k8serrors.IsUnauthorized(err):
		return NewInvalidAuthError(err)
	case k8serrors.IsNotFound(err):
		return NewNotFoundError(err, resourceType)
	case k8serrors.IsForbidden(err):
		return NewForbiddenError(err, resourceType)
	default:
		return err
	}
}

func AsUnprocessableEntity(err error, detail string, errTypes ...ApiError) error {
	if err == nil {
		return nil
	}

	for _, errType := range errTypes {
		// At this point in time the errors in the errType array are downgraded
		// to `ApiError`. This means that pointers to api errors that only
		// embed `apiError` are assignable to each other. Therefore `errors.As`
		// would return `true` and would change the initial value type of the
		// array entry. That is why we need to get the "desiredType" first and
		// compare it to the type that has been set by `errors.As`
		desiredErrType := reflect.ValueOf(errType).Type()

		if !errors.As(err, &errType) {
			continue
		}

		asErrType := reflect.ValueOf(errType).Type()
		if asErrType != desiredErrType {
			continue
		}

		return NewUnprocessableEntityError(errType.Unwrap(), detail)
	}

	return err
}

func ForbiddenAsNotFound(err error) error {
	var forbiddenErr ForbiddenError
	if errors.As(err, &forbiddenErr) {
		return NewNotFoundError(forbiddenErr.Unwrap(), forbiddenErr.ResourceType())
	}
	return err
}

// DropletForbiddenAsNotFound is a special case due to the CF CLI expecting the error message "Droplet not found" exactly instead of the generic case
// https://github.com/cloudfoundry/korifi/issues/965
func DropletForbiddenAsNotFound(err error) error {
	var forbiddenErr ForbiddenError
	if errors.As(err, &forbiddenErr) {
		return NotFoundError{
			apiError{
				cause:      forbiddenErr.Unwrap(),
				title:      "CF-ResourceNotFound",
				detail:     "Droplet not found",
				code:       10010,
				httpStatus: http.StatusNotFound,
			},
		}
	}
	var notFoundErr NotFoundError
	if errors.As(err, &notFoundErr) {
		return NotFoundError{
			apiError{
				cause:      notFoundErr.Unwrap(),
				title:      "CF-ResourceNotFound",
				detail:     "Droplet not found",
				code:       10010,
				httpStatus: http.StatusNotFound,
			},
		}
	}
	return err
}
