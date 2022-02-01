package repositories

import (
	"errors"
	"fmt"
)

type NotFoundError struct {
	Err          error
	ResourceType string
}

func (e NotFoundError) Error() string {
	msg := "not found"
	if e.ResourceType != "" {
		msg = e.ResourceType + " " + msg
	}
	return errMessage(msg, e.Err)
}

func (e NotFoundError) Unwrap() error {
	return e.Err
}

type PermissionDeniedOrNotFoundError struct {
	Err error
}

func (e PermissionDeniedOrNotFoundError) Error() string {
	return errMessage("Resource not found or permission denied", e.Err)
}

func (e PermissionDeniedOrNotFoundError) Unwrap() error {
	return e.Err
}

type ResourceNotFoundError struct {
	Err error
}

func (e ResourceNotFoundError) Error() string {
	return errMessage("Resource not found", e.Err)
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
	return errMessage("Forbidden", e.err)
}

func (e ForbiddenError) Unwrap() error {
	return e.err
}

func IsForbiddenError(err error) bool {
	return errors.As(err, &ForbiddenError{})
}

func errMessage(message string, err error) string {
	if err == nil {
		return message
	}

	return fmt.Sprintf("%s: %v", message, err)
}
