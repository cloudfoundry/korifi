package payloads

import (
	"code.cloudfoundry.org/cf-k8s-controllers/api/repositories"
	rbacv1 "k8s.io/api/rbac/v1"
)

type RoleCreate struct {
	Type          string            `json:"type" validate:"required"`
	Relationships RoleRelationships `json:"relationships" validate:"required"`
}

type UserRelationship struct {
	Data UserRelationshipData `json:"data" validate:"required"`
}

type UserRelationshipData struct {
	Username string `json:"username" validate:"required_without=GUID"`
	GUID     string `json:"guid" validate:"required_without=Username"`
}

type RoleRelationships struct {
	User                     *UserRelationship `json:"user" validate:"required_without=KubernetesServiceAccount"`
	KubernetesServiceAccount *Relationship     `json:"kubernetesServiceAccount" validate:"required_without=User"`
	Space                    *Relationship     `json:"space"`
	Organization             *Relationship     `json:"organization"`
}

func (p RoleCreate) ToMessage() repositories.RoleCreateMessage {
	record := repositories.RoleCreateMessage{
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
		record.User = p.Relationships.User.Data.Username
		if p.Relationships.User.Data.GUID != "" {
			record.User = p.Relationships.User.Data.GUID
		}
	} else {
		record.Kind = rbacv1.ServiceAccountKind
		record.User = p.Relationships.KubernetesServiceAccount.Data.GUID
	}

	return record
}
