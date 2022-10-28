package matchers

import (
	"encoding/json"
	"fmt"

	"code.cloudfoundry.org/korifi/controllers/webhooks"

	//. "github.com/onsi/gomega"

	"github.com/onsi/gomega"
	"github.com/onsi/gomega/gstruct"
	"github.com/onsi/gomega/matchers"
	"github.com/onsi/gomega/types"
)

type beValidationErrorMatcher struct {
	matcher types.GomegaMatcher
}

func BeValidationError(expectedErrorType string, messageMatcher types.GomegaMatcher) types.GomegaMatcher {
	return &beValidationErrorMatcher{
		matcher: &matchers.AndMatcher{
			Matchers: []types.GomegaMatcher{
				gomega.BeAssignableToTypeOf(webhooks.ValidationError{}),
				gstruct.MatchAllFields(gstruct.Fields{
					"Type":    gomega.Equal(expectedErrorType),
					"Message": messageMatcher,
				}),
			},
		},
	}
}

func (m *beValidationErrorMatcher) Match(actual interface{}) (bool, error) {
	validationErr, err := toValidationError(actual)
	if err != nil {
		return false, err
	}
	return m.matcher.Match(validationErr)
}

func (m *beValidationErrorMatcher) FailureMessage(actual interface{}) (message string) {
	validationErr, err := toValidationError(actual)
	if err != nil {
		return err.Error()
	}
	return m.matcher.FailureMessage(validationErr)
}

func (m *beValidationErrorMatcher) NegatedFailureMessage(actual interface{}) (message string) {
	validationErr, err := toValidationError(actual)
	if err != nil {
		return err.Error()
	}
	return m.matcher.NegatedFailureMessage(validationErr)
}

func toValidationError(actual interface{}) (webhooks.ValidationError, error) {
	actualErr, ok := actual.(error)
	if !ok {
		return webhooks.ValidationError{}, fmt.Errorf("%v is not an error", actual)
	}

	var validationErr webhooks.ValidationError
	err := json.Unmarshal([]byte(actualErr.Error()), &validationErr)
	if err != nil {
		return webhooks.ValidationError{}, fmt.Errorf("%v is not a validation error: %w", actualErr.Error(), err)
	}

	return validationErr, nil
}
