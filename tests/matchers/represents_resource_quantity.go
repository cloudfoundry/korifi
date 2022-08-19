package matchers

import (
	"fmt"

	"github.com/onsi/gomega/format"
	"github.com/onsi/gomega/types"
	"k8s.io/apimachinery/pkg/api/resource"
)

type representsResourceQuantityMatcher struct {
	amount   int64
	scaleStr string
}

func RepresentResourceQuantity(amount int64, scaleStr string) types.GomegaMatcher {
	return &representsResourceQuantityMatcher{amount: amount, scaleStr: scaleStr}
}

func (m *representsResourceQuantityMatcher) Match(actual interface{}) (bool, error) {
	if actual == nil {
		return false, nil
	}

	actualQuantity, ok := actual.(*resource.Quantity)
	if !ok {
		return false, fmt.Errorf("%#v is not a *resource.Quantity", actual)
	}

	expectedQuantity, err := m.expectedQuantity()
	if err != nil {
		return false, fmt.Errorf("error parsing expected quantity: %w", err)
	}

	return actualQuantity.Equal(expectedQuantity), nil
}

func (m *representsResourceQuantityMatcher) FailureMessage(actual interface{}) (message string) {
	return format.Message(m.actualString(actual), "to represent ", m.expectedString())
}

func (m *representsResourceQuantityMatcher) NegatedFailureMessage(actual interface{}) (message string) {
	return format.Message(m.actualString(actual), "not to represent ", m.expectedString())
}

func (m *representsResourceQuantityMatcher) expectedQuantity() (resource.Quantity, error) {
	return resource.ParseQuantity(fmt.Sprintf("%d%s", m.amount, m.scaleStr))
}

func (m *representsResourceQuantityMatcher) actualString(actual any) string {
	actualQuantity := actual.(*resource.Quantity)
	return m.formatQuantity(actualQuantity)
}

func (m *representsResourceQuantityMatcher) expectedString() string {
	expectedQuantity, _ := m.expectedQuantity()
	return m.formatQuantity(&expectedQuantity)
}

func (m *representsResourceQuantityMatcher) formatQuantity(quantity *resource.Quantity) string {
	asInt, ok := quantity.AsInt64()
	if !ok {
		return fmt.Sprintf("%s (%f)", quantity.String(), quantity.AsApproximateFloat64())
	}
	return fmt.Sprintf("%s (%d)", quantity.String(), asInt)
}
