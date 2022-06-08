package matchers

import (
	"encoding/json"
	"fmt"
	"reflect"

	"code.cloudfoundry.org/korifi/controllers/webhooks"

	//. "github.com/onsi/gomega"
	"github.com/onsi/gomega/format"
	"github.com/onsi/gomega/types"
)

type representJSONifiedValidationErrorMatcher struct {
	expected webhooks.ValidationError
}

func RepresentJSONifiedValidationError(expected webhooks.ValidationError) types.GomegaMatcher {
	return &representJSONifiedValidationErrorMatcher{
		expected: expected,
	}
}

func (m *representJSONifiedValidationErrorMatcher) Match(actual interface{}) (bool, error) {
	if actual == nil {
		return false, nil
	}

	actualError, isError := actual.(error)
	if !isError {
		return false, fmt.Errorf("%#v is not an error", actual)
	}

	ve := new(webhooks.ValidationError)
	err := json.Unmarshal([]byte(actualError.Error()), ve)
	if err != nil {
		return false, fmt.Errorf("Failed to decode JSON: %s", err.Error())
	}

	return reflect.DeepEqual(*ve, m.expected), nil
}

func (m *representJSONifiedValidationErrorMatcher) FailureMessage(actual interface{}) (message string) {
	return format.Message(actual, "to contain a JSON representation of ", m.expected)
}

func (m *representJSONifiedValidationErrorMatcher) NegatedFailureMessage(actual interface{}) (message string) {
	return format.Message(actual, "not to contain a JSON representation of ", m.expected)
}
