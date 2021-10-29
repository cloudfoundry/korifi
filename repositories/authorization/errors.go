package authorization

import "errors"

type UnauthorizedErr struct{}

func (e UnauthorizedErr) Error() string {
	return "unauthorized"
}

func IsUnauthorized(err error) bool {
	return errors.As(err, &UnauthorizedErr{})
}
