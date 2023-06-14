package validation

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/jellydator/validation"
)

func NotStartWith(prefix string) validation.Rule {
	return validation.NewStringRule(func(value string) bool {
		return !strings.HasPrefix(value, prefix)
	}, fmt.Sprintf("prefix %s is not allowed", prefix))
}

func NotEqual(value string) validation.Rule {
	return validation.NewStringRule(func(actualValue string) bool {
		return actualValue != value
	}, fmt.Sprintf("value %s is not allowed", value))
}

var StrictlyRequired = strictlyRequiredRule{}

type strictlyRequiredRule struct {
	validation.RequiredRule
}

// We wrap tje original validation.RequiredRule in order to workaround
// incorrect zero type check:
// https://github.com/jellydator/validation/blob/44595f5c48dd0da8bbeff0f56ceaa581631e55b1/util.go#L151-L156
func (r strictlyRequiredRule) Validate(value interface{}) error {
	if err := r.RequiredRule.Validate(value); err != nil {
		return err
	}

	if reflect.DeepEqual(value, reflect.Zero(reflect.TypeOf(value)).Interface()) {
		return validation.ErrRequired
	}

	return nil
}
