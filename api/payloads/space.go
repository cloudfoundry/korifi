package payloads

import (
	"code.cloudfoundry.org/cf-k8s-controllers/api/repositories"
)

type SpaceCreate struct {
	Name          string             `json:"name" validate:"required"`
	Relationships SpaceRelationships `json:"relationships" validate:"required"`
	Metadata      Metadata           `json:"metadata"`
}

type SpaceRelationships struct {
	Org Relationship `json:"organization" validate:"required"`
}

func (p SpaceCreate) ToRecord() repositories.SpaceRecord {
	return repositories.SpaceRecord{
		Name:             p.Name,
		OrganizationGUID: p.Relationships.Org.Data.GUID,
		Labels:           p.Metadata.Labels,
		Annotations:      p.Metadata.Annotations,
	}
}
