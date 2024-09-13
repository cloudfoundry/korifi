package compare

import (
	"slices"
	"strings"
)

type SortOrder int

const (
	Ascending  SortOrder = 1
	Descending SortOrder = -1
)

type Sorter[T any] struct {
	comparatorFactory func(string) func(T, T) int
}

func NewSorter[T any](comparatorFactory func(string) func(T, T) int) *Sorter[T] {
	return &Sorter[T]{comparatorFactory: comparatorFactory}
}

func (s Sorter[T]) Sort(records []T, orderBy string) []T {
	field, isDescending := strings.CutPrefix(orderBy, "-")

	sortOrder := Ascending
	if isDescending {
		sortOrder = Descending
	}

	comparator := orderedComparator(sortOrder, s.comparatorFactory(field))
	slices.SortFunc(records, comparator)
	return records
}

func orderedComparator[T any](
	order SortOrder,
	comparator func(T, T) int,
) func(T, T) int {
	return func(t1, t2 T) int {
		return int(order) * comparator(t1, t2)
	}
}
