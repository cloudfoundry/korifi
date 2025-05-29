package descriptors

import (
	"fmt"
	"slices"
	"strings"
)

type Pair struct {
	Guid      string
	Value     any
	ValueType string
}

type PairsList []*Pair

func (p PairsList) Sort(desc bool) {
	if len(p) == 0 {
		return
	}

	valueType := p[0].ValueType
	// TODO: maybe error of the value type is neither string, nor integer (i.e. not supported)
	switch valueType {
	case "integer":
		sortIntegerPairs(p, desc)
	default:
		sortStringPairs(p, desc)
	}
}

func sortStringPairs(pairs []*Pair, desc bool) {
	slices.SortFunc(pairs, func(a, b *Pair) int {
		aValue := fmt.Sprintf("%v", a.Value)
		bValue := fmt.Sprintf("%v", b.Value)

		if desc {
			return strings.Compare(aValue, bValue) * -1
		}
		return strings.Compare(aValue, bValue)
	})
}

func sortIntegerPairs(pairs []*Pair, desc bool) {
	slices.SortFunc(pairs, func(a, b *Pair) int {
		aValue := a.Value.(int)
		bValue := b.Value.(int)

		if desc {
			return bValue - aValue
		}

		return aValue - bValue
	})
}
