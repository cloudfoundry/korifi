package repositories

import "errors"

type NotFoundError struct {
	Err          error
	ResourceType string
}

func (e NotFoundError) Error() string {
	msg := "not found"
	if e.ResourceType != "" {
		msg = e.ResourceType + " " + msg
	}
	if e.Err != nil {
		msg = msg + ": " + e.Err.Error()
	}
	return msg
}

func (e NotFoundError) Unwrap() error {
	return e.Err
}

type PermissionDeniedOrNotFoundError struct {
	Err error
}

func (e PermissionDeniedOrNotFoundError) Error() string {
	return "Resource not found or permission denied."
}

func (e PermissionDeniedOrNotFoundError) Unwrap() error {
	return e.Err
}

type ResourceNotFoundError struct {
	Err error
}

func (e ResourceNotFoundError) Error() string {
	return "Resource not found."
}

func (e ResourceNotFoundError) Unwrap() error {
	return e.Err
}

type ForbiddenError struct {
	err error
}

func NewForbiddenError(err error) ForbiddenError {
	return ForbiddenError{err: err}
}

func (e ForbiddenError) Error() string {
	return "Forbidden"
}

func (e ForbiddenError) Unwrap() error {
	return e.err
}

func IsForbiddenError(err error) bool {
	return errors.As(err, &ForbiddenError{})
}
