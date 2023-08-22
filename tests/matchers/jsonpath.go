package matchers

import (
	"encoding/json"
	"fmt"

	"github.com/PaesslerAG/jsonpath"
	"github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
)

type JSONPathMatcher struct {
	path          string
	expected      types.GomegaMatcher
	jsonPathValue any
	respObj       any
}

func MatchJSONPath(path string, expected any) *JSONPathMatcher {
	matcher := &JSONPathMatcher{path: path}

	if gm, ok := expected.(types.GomegaMatcher); ok {
		matcher.expected = gm
	} else {
		matcher.expected = gomega.Equal(expected)
	}

	return matcher
}

func (m *JSONPathMatcher) Match(actual interface{}) (bool, error) {
	v, err := m.get(actual)
	if err != nil {
		return false, err
	}

	m.jsonPathValue = v

	return m.expected.Match(v)
}

func (m *JSONPathMatcher) get(actual interface{}) (any, error) {
	var bs []byte
	switch a := actual.(type) {
	case []byte:
		bs = a
	case string:
		bs = []byte(a)
	default:
		return false, fmt.Errorf("found %T, expected []byte", actual)
	}

	resp := any(nil)
	err := json.Unmarshal(bs, &resp)
	if err != nil {
		return false, err
	}
	m.respObj = resp

	return jsonpath.Get(m.path, resp)
}

func (m JSONPathMatcher) FailureMessage(actual interface{}) string {
	return fmt.Sprintf(
		"Expected\n\t%s in %#v\nto match: %s",
		m.path,
		m.respObj,
		m.expected.FailureMessage(m.jsonPathValue),
	)
}

func (m JSONPathMatcher) NegatedFailureMessage(actual interface{}) string {
	return fmt.Sprintf(
		"Expected\n\t%s in %#v\nnot to match: %s",
		m.path,
		m.respObj,
		m.expected.NegatedFailureMessage(m.jsonPathValue),
	)
}

type JSONPathErrorMatcher struct {
	JSONPathMatcher
}

func MatchJSONPathError(path string, expected any) *JSONPathErrorMatcher {
	matcher := &JSONPathErrorMatcher{
		JSONPathMatcher: JSONPathMatcher{
			path: path,
		},
	}

	if gm, ok := expected.(types.GomegaMatcher); ok {
		matcher.expected = gm
	} else {
		matcher.expected = gomega.Equal(expected)
	}

	return matcher
}

func (m *JSONPathErrorMatcher) Match(actual interface{}) (bool, error) {
	_, err := m.get(actual)
	m.respObj = err
	return m.expected.Match(err)
}

func (m JSONPathErrorMatcher) FailureMessage(actual interface{}) string {
	return fmt.Sprintf(
		"Expected error for \n\t%s in %s\n to match: %s",
		m.path,
		actual,
		m.expected.FailureMessage(m.respObj),
	)
}

func (m JSONPathErrorMatcher) NegatedFailureMessage(actual interface{}) string {
	return fmt.Sprintf(
		"Expected error for \n\t%s in %s\nnot to match: %s",
		m.path,
		actual,
		m.expected.FailureMessage(m.respObj),
	)
}
