package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"golang.org/x/exp/maps"
)

func printDocForSchema(schema map[string]any, indentLevel int) {
	indentStr := strings.Repeat("  ", indentLevel)
	names := maps.Keys(schema)
	sort.Strings(names)

	for _, name := range names {
		value := schema[name].(map[string]any)
		desc := ""
		if descAny, ok := value["description"]; ok {
			desc = " " + descAny.(string)
		}

		typeStr := value["type"].(string)
		if typeStr == "object" {
			typeStr = ""
		} else {
			// nolint:staticcheck
			typeStr = " (_" + strings.Title(typeStr) + "_)"
		}

		fmt.Printf("%s- `%s`%s:%s\n", indentStr, name, typeStr, desc)
		if value["type"].(string) == "object" {
			printDocForSchema(value["properties"].(map[string]any), indentLevel+1)
		}
	}
}

func main() {
	files := [][2]string{
		{"", "helm/korifi/values.schema.json"},
		{"api", "helm/api/values.schema.json"},
		{"controllers", "helm/controllers/values.schema.json"},
		{"job-task-runner", "helm/job-task-runner/values.schema.json"},
		{"kpack-image-builder", "helm/kpack-image-builder/values.schema.json"},
		{"statefulset-runner", "helm/statefulset-runner/values.schema.json"},
	}

	fmt.Printf(`# Korifi Helm chart

This documents the [Helm](https://helm.sh/) chart for [Korifi](https://github.com/cloudfoundry/korifi).

The chart is a composition of subcharts, one per component, with each individual component configuration nested under a top-level key named after the component itself.
Values under the top-level %[1]sglobal%[1]s key apply to all components.
Each component can be excluded from the deployment by the setting its %[1]sinclude%[1]s value to %[1]sfalse%[1]s.
See [_Customizing the Chart Before Installing_](https://helm.sh/docs/intro/using_helm/#customizing-the-chart-before-installing) for details on how to specify values when installing a Helm chart.

Here are all the values that can be set for the chart:

`, "`")

	for _, f := range files {
		section := f[0]
		file := f[1]

		bs, err := os.ReadFile(file)
		if err != nil {
			panic(err)
		}
		var schema map[string]any
		err = json.Unmarshal(bs, &schema)
		if err != nil {
			panic(err)
		}

		if section != "" {
			fmt.Printf("- `%s`:\n", section)
			printDocForSchema(schema["properties"].(map[string]any), 1)
		} else {
			printDocForSchema(schema["properties"].(map[string]any), 0)
		}
	}
}
