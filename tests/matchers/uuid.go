package matchers

import (
	"fmt"

	"github.com/google/uuid"
	"github.com/onsi/gomega/format"
	"github.com/onsi/gomega/types"
)

type validUUIDMatcher struct{}

func BeValidUUID() types.GomegaMatcher {
	return &validUUIDMatcher{}
}

func (m *validUUIDMatcher) Match(actual interface{}) (bool, error) {
	actualString, isString := actual.(string)
	if !isString {
		return false, fmt.Errorf("%#v is not a string", actual)
	}

	_, err := uuid.Parse(actualString)
	return err == nil, nil
}

func (m *validUUIDMatcher) FailureMessage(actual interface{}) (message string) {
	return format.Message(actual, "to be a valud UUID")
}

func (m *validUUIDMatcher) NegatedFailureMessage(actual interface{}) (message string) {
	return format.Message(actual, "not to be a valid UUID")
}
