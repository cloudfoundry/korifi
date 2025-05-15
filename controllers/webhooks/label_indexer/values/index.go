package values

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"code.cloudfoundry.org/korifi/tools"
	"github.com/PaesslerAG/jsonpath"
)

type (
	IndexValueFunc func(map[string]any) (*string, error)
)

func JSONValue(path string) IndexValueFunc {
	return func(obj map[string]any) (*string, error) {
		value, err := jsonpath.Get(path, obj)
		if err != nil {
			if strings.HasPrefix(err.Error(), "unknown key") {
				return nil, nil
			}
			return nil, fmt.Errorf("failed to get value from JSONPath %s: %w", path, err)
		}

		return marshal(value)
	}
}

func SingleValue(prev IndexValueFunc) IndexValueFunc {
	return func(obj map[string]any) (*string, error) {
		jsonString, err := prev(obj)
		if err != nil {
			return nil, err
		}
		if jsonString == nil {
			return nil, nil
		}

		var array []any
		if err := json.Unmarshal([]byte(*jsonString), &array); err != nil {
			return nil, fmt.Errorf("failed to unmarshal value %s: %w", *jsonString, err)
		}

		if len(array) > 1 {
			return nil, fmt.Errorf("expected single value, got array %v", array)
		}

		if len(array) == 0 {
			return nil, nil
		}

		return marshal(array[0])
	}
}

func IsEmptyValue(prev IndexValueFunc) IndexValueFunc {
	return func(obj map[string]any) (*string, error) {
		jsonString, err := prev(obj)
		if err != nil {
			return nil, err
		}
		if jsonString == nil {
			return nil, nil
		}

		var array []any
		if err := json.Unmarshal([]byte(*jsonString), &array); err != nil {
			return nil, fmt.Errorf("failed to unmarshal value %s: %w", *jsonString, err)
		}

		if len(array) == 0 {
			return tools.PtrTo("true"), nil
		}

		return tools.PtrTo("false"), nil
	}
}

func Unquote(prev IndexValueFunc) IndexValueFunc {
	return func(obj map[string]any) (*string, error) {
		prevValue, err := prev(obj)
		if err != nil {
			return nil, err
		}

		if prevValue == nil {
			return nil, nil
		}

		unquoted, err := strconv.Unquote(*prevValue)
		if err != nil {
			return nil, fmt.Errorf("failed to unquote value %s: %w", *prevValue, err)
		}

		return tools.PtrTo(unquoted), nil
	}
}

func SHA224(prev IndexValueFunc) IndexValueFunc {
	return func(obj map[string]any) (*string, error) {
		prevValue, err := prev(obj)
		if err != nil {
			return nil, err
		}
		if prevValue == nil {
			return nil, nil
		}

		return tools.PtrTo(tools.EncodeValueToSha224(*prevValue)), nil
	}
}

func EmptyValue() IndexValueFunc {
	return func(_ map[string]any) (*string, error) {
		return tools.PtrTo(""), nil
	}
}

func marshal(value any) (*string, error) {
	valueBytes, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal value %v: %w", value, err)
	}

	return tools.PtrTo(string(valueBytes)), nil
}
