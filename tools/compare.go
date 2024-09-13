package tools

import "time"

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
