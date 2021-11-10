package payloads

import "code.cloudfoundry.org/cf-k8s-controllers/api/repositories"

type RoleCreate struct {
	Type          string            `json:"type" validate:"required"`
	Relationships RoleRelationships `json:"relationships" validate:"required"`
}

type RoleRelationships struct {
	User         Relationship  `json:"user" validate:"required"`
	Space        *Relationship `json:"space"`
	Organization *Relationship `json:"organization"`
}

func (p RoleCreate) ToRecord() repositories.RoleRecord {
	record := repositories.RoleRecord{
		Type: p.Type,
		User: p.Relationships.User.Data.GUID,
	}

	if p.Relationships.Space != nil {
		record.Space = p.Relationships.Space.Data.GUID
	}

	if p.Relationships.Organization != nil {
		record.Org = p.Relationships.Organization.Data.GUID
	}

	return record
}
