package params

import (
	"fmt"
	"net/url"
	"strings"

	"code.cloudfoundry.org/korifi/api/payloads/validation"
	jellidation "github.com/jellydator/validation"
)

type IncludeResourceRule struct {
	RelationshipPath []string
	Fields           []string
}

func (r IncludeResourceRule) Validate() error {
	return jellidation.ValidateStruct(&r,
		jellidation.Field(&r.RelationshipPath, jellidation.Each(validation.OneOf("service_offering", "service_broker", "space", "organization"))),
		jellidation.Field(&r.Fields, jellidation.Each(validation.OneOf("guid", "name"))),
	)
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
