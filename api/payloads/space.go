package payloads

import (
	"code.cloudfoundry.org/cf-k8s-controllers/api/repositories"
)

const orgPrefix = "cforg-"

type SpaceCreate struct {
	Name          string             `json:"name" validate:"required"`
	Relationships SpaceRelationships `json:"relationships" validate:"required"`
	Metadata      Metadata           `json:"metadata"`
}

type SpaceRelationships struct {
	Org Relationship `json:"organization" validate:"required"`
}

func (p SpaceCreate) ToMessage(imageRegistryCredentialSecret string) repositories.CreateSpaceMessage {
	return repositories.CreateSpaceMessage{
		Name:                     p.Name,
		OrganizationGUID:         orgPrefix + p.Relationships.Org.Data.GUID,
		ImageRegistryCredentials: imageRegistryCredentialSecret,
	}
}
