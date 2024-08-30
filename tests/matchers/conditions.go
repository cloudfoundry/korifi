package matchers

import (
	"fmt"

	. "github.com/onsi/gomega" //lint:ignore ST1001 this is a test file
	"github.com/onsi/gomega/format"
	"github.com/onsi/gomega/types"
)

func HasType(matcher types.GomegaMatcher) types.GomegaMatcher {
	return &conditionMatcher{field: "Type", matcher: matcher}
}

func HasStatus(matcher types.GomegaMatcher) types.GomegaMatcher {
	return &conditionMatcher{field: "Status", matcher: matcher}
}

func HasReason(matcher types.GomegaMatcher) types.GomegaMatcher {
	return &conditionMatcher{field: "Reason", matcher: matcher}
}

func HasMessage(matcher types.GomegaMatcher) types.GomegaMatcher {
	return &conditionMatcher{field: "Message", matcher: matcher}
}

func HasObservedGeneration(matcher types.GomegaMatcher) types.GomegaMatcher {
	return &conditionMatcher{field: "ObservedGeneration", matcher: matcher}
}

type conditionMatcher struct {
	field   string
	matcher types.GomegaMatcher
}

func (m *conditionMatcher) Match(actual interface{}) (bool, error) {
	return HaveField(m.field, m.matcher).Match(actual)
}

func (m *conditionMatcher) FailureMessage(actual interface{}) (message string) {
	return format.Message(actual, fmt.Sprintf("to have field %q ", m.field), m.matcher.FailureMessage(actual))
}

func (m *conditionMatcher) NegatedFailureMessage(actual interface{}) (message string) {
	return format.Message(actual, fmt.Sprintf("not to have field %q ", m.field), m.matcher.NegatedFailureMessage(actual))
}
