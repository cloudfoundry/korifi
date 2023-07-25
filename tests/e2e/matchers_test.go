package e2e_test

import (
	"encoding/json"
	"fmt"
	"io"

	"code.cloudfoundry.org/korifi/tests/helpers"
	"github.com/go-resty/resty/v2"
	"github.com/onsi/gomega"
	"github.com/onsi/gomega/format"
	"github.com/onsi/gomega/gstruct"
	"github.com/onsi/gomega/matchers"
	"github.com/onsi/gomega/types"
)

func HaveRestyStatusCode(expected int) types.GomegaMatcher {
	return &haveRestyStatusCode{
		expected: expected,
	}
}

func HaveRestyBody(expected interface{}) types.GomegaMatcher {
	switch e := expected.(type) {
	case types.GomegaMatcher:
		return &haveRestyBody{matcher: e}
	default:
		return &haveRestyBody{matcher: &matchers.EqualMatcher{Expected: expected}}
	}
}

func HaveRestyHeaderWithValue(key string, value interface{}) types.GomegaMatcher {
	return haveRestyHeaderWithValue{
		key:   key,
		value: value,
	}
}

func BeAValidJSONObject() types.GomegaMatcher {
	return new(validJSONObjectMatcher)
}

func HaveRelationship(relationshipName, relationshipKey, relationshipValue string) types.GomegaMatcher {
	return &haveRelationshipMatcher{
		relationshipName:  relationshipName,
		relationshipKey:   relationshipKey,
		relationshipValue: relationshipValue,
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

type haveRelationshipMatcher struct {
	relationshipName  string
	relationshipKey   string
	relationshipValue string
}

func (m *haveRelationshipMatcher) Match(actual interface{}) (bool, error) {
	if actual == nil {
		return false, nil
	}

	actualResource, ok := actual.(typedResource)
	if !ok {
		return false, fmt.Errorf("%#v is not a e2e_test.typedResource", actual)
	}

	rel, ok := actualResource.Relationships[m.relationshipName]
	if !ok {
		return false, fmt.Errorf("the missing relationship is %s", m.relationshipName)
	}

	return m.dataHaveKeyMatcher().Match(rel.Data)
}

func (m *haveRelationshipMatcher) FailureMessage(actual interface{}) (message string) {
	return format.Message(
		actual,
		"to have relationship ",
		fmt.Sprintf("%s:%s:%s \n%s", m.relationshipName, m.relationshipKey, m.relationshipValue, m.dataHaveKeyMatcher().FailureMessage(actual)),
	)
}

func (m *haveRelationshipMatcher) NegatedFailureMessage(actual interface{}) (message string) {
	return format.Message(
		actual,
		"not to have relationship ",
		fmt.Sprintf("%s:%s:%s \n%s", m.relationshipName, m.relationshipKey, m.relationshipValue, m.dataHaveKeyMatcher().FailureMessage(actual)),
	)
}

func (m *haveRelationshipMatcher) dataHaveKeyMatcher() types.GomegaMatcher {
	return gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
		m.relationshipKey: gomega.Equal(m.relationshipValue),
	})
}

type validJSONObjectMatcher struct{}

func (matcher *validJSONObjectMatcher) Match(actual interface{}) (bool, error) {
	decoded := map[string]string{}
	var body []byte
	switch b := actual.(type) {
	case []byte:
		body = b
	case string:
		body = []byte(b)
	case fmt.Stringer:
		body = []byte(b.String())
	case io.Reader:
		var err error
		body, err = io.ReadAll(b)
		if err != nil {
			return false, fmt.Errorf("error reading from %#v: %w", b, err)
		}
	default:
		return false, fmt.Errorf("can't convert %#v to be a []byte", actual)
	}

	err := json.Unmarshal(body, &decoded)
	if err != nil {
		return false, fmt.Errorf("error unmarshaling %s: %w", body, err)
	}
	return true, nil
}

func (matcher *validJSONObjectMatcher) FailureMessage(actual interface{}) (message string) {
	return fmt.Sprintf("Expected\n\t%#v\nto contain valid JSON", actual)
}

func (matcher *validJSONObjectMatcher) NegatedFailureMessage(actual interface{}) (message string) {
	return fmt.Sprintf("Expected\n\t%#v\nnot to contain valid JSON", actual)
}
