package authorization

import "errors"

type InvalidAuthError struct{}

func (e InvalidAuthError) Error() string {
	return "unauthorized"
}

func IsInvalidAuth(err error) bool {
	return errors.Is(err, InvalidAuthError{})
}

type NotAuthenticatedError struct{}

func (e NotAuthenticatedError) Error() string {
	return "not authenticated"
}

func IsNotAuthenticated(err error) bool {
	return errors.Is(err, NotAuthenticatedError{})
}
