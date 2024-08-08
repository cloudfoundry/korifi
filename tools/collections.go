package tools

import (
	"cmp"
	"slices"
)

func Uniq[S ~[]E, E cmp.Ordered](elements S) S {
	slices.Sort(elements)
	return slices.Compact(elements)
}

func EmptyOrContains[S ~[]E, E comparable](elements S, e E) bool {
	if len(elements) == 0 {
		return true
	}

	return slices.Contains(elements, e)
}

func NilOrEquals[E comparable](value *E, expectedValue E) bool {
	if value == nil {
		return true
	}

	return expectedValue == *value
}
