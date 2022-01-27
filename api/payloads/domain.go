package payloads

import (
	"code.cloudfoundry.org/cf-k8s-controllers/api/repositories"
)

type DomainList struct {
	Names *string `schema:"names"`
}

func (d *DomainList) ToMessage() repositories.ListDomainsMessage {
	return repositories.ListDomainsMessage{
		Names: parseArrayParam(d.Names),
	}
}

func (d *DomainList) SupportedFilterKeys() []string {
	return []string{"names"}
}
