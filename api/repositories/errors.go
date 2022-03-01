package repositories

import (
	"errors"
	"fmt"
)

type repoError struct {
	err          error
	resourceType string
	message      string
}

func (e repoError) Error() string {
	msg := e.message
	if e.resourceType != "" {
		msg = e.resourceType + " " + msg
	}

	if e.err == nil {
		return msg
	}

	return fmt.Sprintf("%s: %v", msg, e.err)
}

func (e repoError) Unwrap() error {
	return e.err
}

func (e repoError) ResourceType() string {
	return e.resourceType
}

type NotFoundError struct {
	repoError
}

func NewNotFoundError(resourceType string, baseError error) NotFoundError {
	return NotFoundError{
		repoError: repoError{err: baseError, resourceType: resourceType, message: "not found"},
	}
}

func IsNotFoundError(err error) bool {
	return errors.As(err, &NotFoundError{})
}

type ForbiddenError struct {
	repoError
}

func NewForbiddenError(resourceType string, baseError error) ForbiddenError {
	return ForbiddenError{
		repoError: repoError{err: baseError, resourceType: resourceType, message: "forbidden"},
	}
}

func IsForbiddenError(err error) bool {
	return errors.As(err, &ForbiddenError{})
}

type DuplicateError struct {
	repoError
}

func NewDuplicateError(resourceType string, baseError error) DuplicateError {
	return DuplicateError{
		repoError: repoError{err: baseError, resourceType: resourceType, message: "duplicate"},
	}
}

func IsDuplicateError(err error) bool {
	return errors.As(err, &DuplicateError{})
}
