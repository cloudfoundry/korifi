package singleton

import (
	"fmt"

	apierrors "code.cloudfoundry.org/korifi/api/errors"
)

type resource interface {
	GetResourceType() string
}

func Get[T resource](objects []T) (T, error) {
	var t T

	if len(objects) == 0 {
		return t, apierrors.NewNotFoundError(nil, fmt.Sprintf("%T", t))
	}

	if len(objects) > 1 {
		return t, apierrors.NewUnprocessableEntityError(nil, fmt.Sprintf("duplicate %q objects exist", objects[0].GetResourceType()))
	}

	return objects[0], nil
}
