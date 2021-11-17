package payloads

import (
	"code.cloudfoundry.org/cf-k8s-controllers/api/repositories"
	rbacv1 "k8s.io/api/rbac/v1"
)

type RoleCreate struct {
	Type          string            `json:"type" validate:"required"`
	Relationships RoleRelationships `json:"relationships" validate:"required"`
}

type RoleRelationships struct {
	User                     *Relationship `json:"user" validate:"required_without=KubernetesServiceAccount"`
	KubernetesServiceAccount *Relationship `json:"kubernetesServiceAccount" validate:"required_without=User"`
	Space                    *Relationship `json:"space"`
	Organization             *Relationship `json:"organization"`
}

func (p RoleCreate) ToRecord() repositories.RoleRecord {
	record := repositories.RoleRecord{
		Type: p.Type,
	}

	if p.Relationships.Space != nil {
		record.Space = p.Relationships.Space.Data.GUID
	}

	if p.Relationships.Organization != nil {
		record.Org = p.Relationships.Organization.Data.GUID
	}

	if p.Relationships.User != nil {
		record.Kind = rbacv1.UserKind
		record.User = p.Relationships.User.Data.GUID
	} else {
		record.Kind = rbacv1.ServiceAccountKind
		record.User = p.Relationships.KubernetesServiceAccount.Data.GUID
	}

	return record
}
