package main

import (
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"slices"
	"sort"
	"strings"

	"github.com/BooleanCat/go-functional/v2/it"
	"github.com/BooleanCat/go-functional/v2/it/itx"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

func printDocForSchema(schema map[string]any, indentLevel int) {
	indentStr := strings.Repeat("  ", indentLevel)
	names := slices.Collect(maps.Keys(schema))
	sort.Slice(names, func(a, b int) bool {
		if names[a] == "global" {
			return true
		}
		if names[b] == "global" {
			return false
		}
		return names[a] < names[b]
	})

	for _, name := range names {
		value := schema[name].(map[string]any)
		desc := ""
		if descAny, ok := value["description"]; ok {
			desc = " " + descAny.(string)
		}

		typeStr := normalizeType(value["type"])
		if typeStr == "object" {
			typeStr = ""
		} else {
			typeStr = " (_" + cases.Title(language.AmericanEnglish).String(typeStr) + "_)"
		}

		fmt.Printf("%s- `%s`%s:%s\n", indentStr, name, typeStr, desc)
		if normalizeType(value["type"]) == "object" {
			properties, hasProperties := value["properties"]
			if hasProperties {
				printDocForSchema(properties.(map[string]any), indentLevel+1)
			}
		}
	}
}

func normalizeType(t any) string {
	typeString, isString := t.(string)
	if isString {
		return typeString
	}

	typeArr := t.([]any)
	types := slices.Collect(it.Exclude(itx.FromSlice(typeArr), func(v any) bool {
		return v == "null"
	}))
	if len(types) != 1 {
		panic(fmt.Errorf("unsupported type: %v", t))
	}

	return normalizeType(types[0])
}

func main() {
	fmt.Printf(`# Korifi Helm chart

This documents the [Helm](https://helm.sh/) chart for [Korifi](https://github.com/cloudfoundry/korifi).

The configuration for each individual component is nested under a top-level key named after the component itself.
Values at the top-level apply to all components.

Each component can be excluded from the deployment by the setting its %[1]sinclude%[1]s value to %[1]sfalse%[1]s.
See [_Customizing the Chart Before Installing_](https://helm.sh/docs/intro/using_helm/#customizing-the-chart-before-installing) for details on how to specify values when installing a Helm chart.

Here are all the values that can be set for the chart:

`, "`")

	bs, err := os.ReadFile("helm/korifi/values.schema.json")
	if err != nil {
		panic(err)
	}
	var schema map[string]any
	err = json.Unmarshal(bs, &schema)
	if err != nil {
		panic(err)
	}

	printDocForSchema(schema["properties"].(map[string]any), 0)
}
