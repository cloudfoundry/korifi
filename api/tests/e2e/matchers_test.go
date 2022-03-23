package e2e_test

import (
	"fmt"

	"code.cloudfoundry.org/cf-k8s-controllers/api/tests/e2e/helpers"
	"github.com/go-resty/resty/v2"
	"github.com/onsi/gomega/format"
	"github.com/onsi/gomega/matchers"
	"github.com/onsi/gomega/types"
)

func HaveRestyStatusCode(expected int) types.GomegaMatcher {
	return &haveRestyStatusCode{
		expected: expected,
	}
}

type haveRestyStatusCode struct {
	expected int
}

func (matcher *haveRestyStatusCode) Match(actual interface{}) (bool, error) {
	response, ok := actual.(*resty.Response)
	if !ok {
		return false, fmt.Errorf("HaveRestyStatusCode matcher expects a resty.Response")
	}

	return response.StatusCode() == matcher.expected, nil
}

func (matcher *haveRestyStatusCode) FailureMessage(actual interface{}) string {
	response, ok := actual.(*resty.Response)
	if !ok {
		return fmt.Sprintf("%v is not a resty.Response", actual)
	}

	return format.Message(helpers.NewActualRestyResponse(response), "to have HTTP Status code", matcher.expected)
}

func (matcher *haveRestyStatusCode) NegatedFailureMessage(actual interface{}) string {
	response, ok := actual.(*resty.Response)
	if !ok {
		return fmt.Sprintf("%v is not a resty.Response", actual)
	}

	return format.Message(helpers.NewActualRestyResponse(response), "not to have HTTP Status code", matcher.expected)
}

func HaveRestyBody(expected interface{}) types.GomegaMatcher {
	switch e := expected.(type) {
	case types.GomegaMatcher:
		return &haveRestyBody{matcher: e}
	default:
		return &haveRestyBody{matcher: &matchers.EqualMatcher{Expected: expected}}
	}
}

type haveRestyBody struct {
	matcher types.GomegaMatcher
}

func (m *haveRestyBody) Match(actual interface{}) (bool, error) {
	response, ok := actual.(*resty.Response)
	if !ok {
		return false, fmt.Errorf("%v is not a resty.Response", actual)
	}

	return m.matcher.Match(response.Body())
}

func (m *haveRestyBody) FailureMessage(actual interface{}) string {
	response, ok := actual.(*resty.Response)
	if !ok {
		return fmt.Sprintf("%v is not a resty.Response", actual)
	}

	return format.Message(helpers.NewActualRestyResponse(response), "to have body", m.matcher)
}

func (m *haveRestyBody) NegatedFailureMessage(actual interface{}) string {
	response, ok := actual.(*resty.Response)
	if !ok {
		return fmt.Sprintf("%v is not a resty.Response", actual)
	}

	return format.Message(helpers.NewActualRestyResponse(response), "not to have body", m.matcher)
}

func HaveRestyHeaderWithValue(key string, value interface{}) types.GomegaMatcher {
	return haveRestyHeaderWithValue{
		key:   key,
		value: value,
	}
}

type haveRestyHeaderWithValue struct {
	key   string
	value interface{}
}

func (m haveRestyHeaderWithValue) Match(actual interface{}) (bool, error) {
	response, ok := actual.(*resty.Response)
	if !ok {
		return false, fmt.Errorf("%v is not a resty.Response", actual)
	}

	hdrVal := response.Header().Get(m.key)

	switch t := m.value.(type) {
	case string:
		return hdrVal == t, nil
	case types.GomegaMatcher:
		return t.Match(hdrVal)
	default:
		return false, fmt.Errorf("expected a string or a matcher, got %T", m.value)
	}
}

func (m haveRestyHeaderWithValue) FailureMessage(actual interface{}) string {
	response, ok := actual.(*resty.Response)
	if !ok {
		return fmt.Sprintf("%v is not a resty.Response", actual)
	}

	hdrVal := response.Header().Get(m.key)
	var matcher types.GomegaMatcher
	switch t := m.value.(type) {
	case string:
		matcher = &matchers.EqualMatcher{Expected: hdrVal}
	case types.GomegaMatcher:
		matcher = t
	default:
		return "invalid matcher"
	}

	return format.Message(helpers.NewActualRestyResponse(response), fmt.Sprintf("to have header %q with value matching", m.key), matcher.FailureMessage(hdrVal))
}

func (m haveRestyHeaderWithValue) NegatedFailureMessage(actual interface{}) string {
	response, ok := actual.(*resty.Response)
	if !ok {
		return fmt.Sprintf("%v is not a resty.Response", actual)
	}

	hdrVal := response.Header().Get(m.key)
	var matcher types.GomegaMatcher
	switch t := m.value.(type) {
	case string:
		matcher = &matchers.EqualMatcher{Expected: hdrVal}
	case types.GomegaMatcher:
		matcher = t
	default:
		return "invalid matcher"
	}

	return format.Message(helpers.NewActualRestyResponse(response), fmt.Sprintf("not to have header %q with value matching", m.key), matcher.FailureMessage(hdrVal))
}
