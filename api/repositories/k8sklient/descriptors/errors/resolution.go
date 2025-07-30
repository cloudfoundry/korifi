package errors

import (
	"errors"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime/schema"
)

type objectResolutionError struct {
	guid string
	gvk  schema.GroupVersionKind
}

func (e objectResolutionError) Error() string {
	return fmt.Sprintf("item not found: guid %q, gvk %q", e.guid, e.gvk)
}

func NewObjectResolutionError(guid string, gvk schema.GroupVersionKind) error {
	return objectResolutionError{
		guid: guid,
		gvk:  gvk,
	}
}

func IsObjectResolutionError(err error) bool {
	return errors.As(err, new(objectResolutionError))
}
