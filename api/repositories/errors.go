package repositories

import (
	"errors"
	"fmt"
)

type repoError struct {
	err          error
	resourceType string
}

func (e repoError) error(msg string) string {
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

type ForbiddenError struct {
	repoError
}

func NewNotFoundError(resourceType string, baseError error) NotFoundError {
	return NotFoundError{
		repoError: repoError{err: baseError, resourceType: resourceType},
	}
}

func NewForbiddenError(resourceType string, baseError error) ForbiddenError {
	return ForbiddenError{
		repoError: repoError{err: baseError, resourceType: resourceType},
	}
}

func (e NotFoundError) Error() string {
	return e.error("not found")
}

func (e ForbiddenError) Error() string {
	return e.error("forbidden")
}

func IsForbiddenError(err error) bool {
	return errors.As(err, &ForbiddenError{})
}
