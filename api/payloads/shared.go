package payloads

import "strings"

func ParseArrayParam(arrayParam string) []string {
	if arrayParam == "" {
		return []string{}
	}

	elements := strings.Split(arrayParam, ",")
	for i, e := range elements {
		elements[i] = strings.TrimSpace(e)
	}

	return elements
}
