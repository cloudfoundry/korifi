package message

import "code.cloudfoundry.org/cf-k8s-api/repositories"

type CreatePackageMessage struct {
	Type          string                `json:"type" validate:"required,oneof='bits'"`
	Relationships *PackageRelationships `json:"relationships" validate:"required"`
}

type PackageRelationships struct {
	App *Relationship `json:"app" validate:"required"`
}

func (m CreatePackageMessage) ToRecord(spaceGUID string) repositories.PackageCreate {
	return repositories.PackageCreate{
		Type:      m.Type,
		AppGUID:   m.Relationships.App.Data.GUID,
		SpaceGUID: spaceGUID,
	}
}
