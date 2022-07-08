package payloads

import (
	"code.cloudfoundry.org/korifi/api/repositories"
)

type DomainList struct {
	Names *string `schema:"names"`
}

func (d *DomainList) ToMessage() repositories.ListDomainsMessage {
	return repositories.ListDomainsMessage{
		Names: ParseArrayParam(d.Names),
	}
}

func (d *DomainList) SupportedKeys() []string {
	return []string{"names"}
}
