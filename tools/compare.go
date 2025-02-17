package tools

import (
	"reflect"
	"time"
)

func CompareTimePtr(t1, t2 *time.Time) int {
	return ZeroIfNil(t1).Compare(ZeroIfNil(t2))
}

func ZeroIfNil[T any, PT *T](value PT) T {
	if value != nil {
		return *value
	}

	var result T
	return result
}

func IfZero[E comparable](v E, ret E) E {
	if reflect.ValueOf(v).IsZero() {
		return ret
	}

	return v
}

func IfNil[T any, PT *T](v PT, ret PT) PT {
	if v == nil {
		return ret
	}

	return v
}

func InsertOrUpdate[K comparable, V any, PV *V](m map[K]V, key K, modifyFunc func(PV)) {
	value := m[key]
	modifyFunc(&value)
	m[key] = value
}
