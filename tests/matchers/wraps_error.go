package matchers

import (
	"errors"
	"fmt"

	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/format"
	"github.com/onsi/gomega/types"
)

type wrapErrorAssignableToTypeOfMatcher struct {
	expected error
}

func WrapErrorAssignableToTypeOf(expected error) types.GomegaMatcher {
	return &wrapErrorAssignableToTypeOfMatcher{expected: expected}
}

func (m *wrapErrorAssignableToTypeOfMatcher) Match(actual interface{}) (bool, error) {
	if actual == nil {
		return false, nil
	}

	actualError, isError := actual.(error)
	if !isError {
		return false, fmt.Errorf("%#v is not an error", actual)
	}

	matches, err := BeAssignableToTypeOf(m.expected).Match(actual)
	if err != nil {
		return false, err
	}
	if matches {
		return true, nil
	}
	return m.Match(errors.Unwrap(actualError))
}

func (m *wrapErrorAssignableToTypeOfMatcher) FailureMessage(actual interface{}) (message string) {
	return format.Message(actual, "to wrap error assignable to type of ", m.expected)
}

func (m *wrapErrorAssignableToTypeOfMatcher) NegatedFailureMessage(actual interface{}) (message string) {
	return format.Message(actual, "not to wrap error assignable to type of ", m.expected)
}
