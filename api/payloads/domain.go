package payloads

import (
	"strings"

	"code.cloudfoundry.org/cf-k8s-controllers/api/repositories"
)

type DomainList struct {
	Names string `schema:"names"`
}

func (d *DomainList) ToMessage() repositories.DomainListMessage {
	return repositories.DomainListMessage{
		Names: parseArrayParam(d.Names),
	}
}

func parseArrayParam(arrayParam string) []string {
	if arrayParam == "" {
		return []string{}
	}

	elements := strings.Split(arrayParam, ",")
	for i, e := range elements {
		elements[i] = strings.TrimSpace(e)
	}
	return elements
}
