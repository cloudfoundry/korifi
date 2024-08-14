package params

import (
	"fmt"
	"net/url"
	"strings"
)

type IncludeResourceRule struct {
	RelationshipPath []string
	Fields           []string
}

func ParseFields(values url.Values) []IncludeResourceRule {
	includes := []IncludeResourceRule{}
	fmt.Printf("values = %+v\n", values)

	for param, values := range values {
		field, isField := strings.CutPrefix(param, "fields")
		if !isField {
			continue
		}

		field = strings.Trim(field, "[]")

		includes = append(includes, IncludeResourceRule{
			RelationshipPath: strings.Split(field, "."),
			Fields:           strings.Split(values[0], ","),
		})
	}

	return includes
}

func ParseIncludes(values url.Values) []IncludeResourceRule {
	includes := []IncludeResourceRule{}

	for param, values := range values {
		if param != "include" {
			continue
		}

		for _, value := range values {
			includes = append(includes, IncludeResourceRule{
				RelationshipPath: strings.Split(value, "."),
				Fields:           []string{},
			})
		}
	}

	return includes
}
