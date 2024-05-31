package matchers

import (
	"encoding/json"
	"fmt"

	"code.cloudfoundry.org/korifi/controllers/webhooks/validation"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"

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
				gomega.BeAssignableToTypeOf(validation.ValidationError{}),
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

func toValidationError(actual interface{}) (validation.ValidationError, error) {
	actualErr, ok := actual.(*k8serrors.StatusError)
	if !ok {
		return validation.ValidationError{}, fmt.Errorf("%v is not a status error", actual)
	}

	var validationErr validation.ValidationError
	err := json.Unmarshal([]byte(actualErr.Status().Reason), &validationErr)
	if err != nil {
		return validation.ValidationError{}, fmt.Errorf("%v is not a validation error: %w", actualErr.Error(), err)
	}

	return validationErr, nil
}
