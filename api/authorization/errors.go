package authorization

import "errors"

type InvalidAuthError struct {
	Err error
}

func (e InvalidAuthError) Error() string {
	return "unauthorized: " + e.Err.Error()
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
