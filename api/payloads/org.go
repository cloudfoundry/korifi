package payloads

import (
	"code.cloudfoundry.org/cf-k8s-controllers/api/repositories"
)

type OrgCreate struct {
	Name      string   `json:"name" validate:"required"`
	Suspended bool     `json:"suspended"`
	Metadata  Metadata `json:"metadata"`
}

func (p OrgCreate) ToRecord() repositories.OrgRecord {
	return repositories.OrgRecord{
		Name:        p.Name,
		Suspended:   p.Suspended,
		Labels:      p.Metadata.Labels,
		Annotations: p.Metadata.Annotations,
	}
}
