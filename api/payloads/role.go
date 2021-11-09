package payloads

import "code.cloudfoundry.org/cf-k8s-controllers/api/repositories"

type RoleCreate struct {
	Type          string            `json:"type" validate:"required"`
	Relationships RoleRelationships `json:"relationships" validate:"required"`
}

type RoleRelationships struct {
	User  Relationship `json:"user" validate:"required"`
	Space Relationship `json:"space" validate:"required"`
}

func (p RoleCreate) ToRecord() repositories.RoleRecord {
	return repositories.RoleRecord{
		Type:  p.Type,
		User:  p.Relationships.User.Data.GUID,
		Space: p.Relationships.Space.Data.GUID,
	}
}
