package repositories

import (
	"errors"
	"fmt"
)

func NewNotFoundError(resourceType string, baseError error) NotFoundError {
	return NotFoundError{
		Err:          baseError,
		ResourceType: resourceType,
	}
}

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
