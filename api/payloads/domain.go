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
		Names: strings.Split(strings.TrimSpace(d.Names), ","),
	}
}
