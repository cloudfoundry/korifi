package payloads

import "code.cloudfoundry.org/cf-k8s-api/repositories"

type PackageCreate struct {
	Type          string                `json:"type" validate:"required,oneof='bits'"`
	Relationships *PackageRelationships `json:"relationships" validate:"required"`
}

type PackageRelationships struct {
	App *Relationship `json:"app" validate:"required"`
}

func (m PackageCreate) ToMessage(spaceGUID string) repositories.PackageCreateMessage {
	return repositories.PackageCreateMessage{
		Type:      m.Type,
		AppGUID:   m.Relationships.App.Data.GUID,
		SpaceGUID: spaceGUID,
	}
}
