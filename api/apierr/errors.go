package apierr

import (
	"fmt"
	"net/http"
)

type ApiError interface {
	Detail() string
	Title() string
	Code() int
	HttpStatus() int
}

type apiError struct {
	cause      error
	detail     string
	title      string
	code       int
	httpStatus int
}

func (e apiError) Error() string {
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

// ------
type NotFoundError struct {
	apiError
}

func NewNotFoundError(cause error, resourceType string) NotFoundError {
	return NotFoundError{
		apiError{
			cause:      cause,
			title:      "CF-ResourceNotFound",
			detail:     fmt.Sprintf("%s not found", resourceType),
			code:       10010,
			httpStatus: http.StatusNotFound,
		},
	}
}

// ------
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

// ------
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

// -----
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

// -----
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

// -----
type MessageParseError struct {
	apiError
}

func NewMessageParseError(cause error) UnprocessableEntityError {
	return UnprocessableEntityError{
		apiError{
			cause:      cause,
			title:      "CF-MessageParseError",
			detail:     "Request invalid due to parse error: invalid request body",
			code:       1001,
			httpStatus: http.StatusBadRequest,
		},
	}
}
