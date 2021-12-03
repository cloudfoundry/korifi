package payloads

import "code.cloudfoundry.org/cf-k8s-controllers/api/repositories"

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

type PackageListQueryParameters struct {
	AppGUIDs string `schema:"app_guids"`

	// Below parameters are ignored, but must be included to ignore as query parameters
	OrderBy string `schema:"order_by"`
	PerPage string `schema:"per_page"`
}

func (p *PackageListQueryParameters) ToMessage() repositories.PackageListMessage {
	return repositories.PackageListMessage{
		AppGUIDs: parseArrayParam(p.AppGUIDs),
	}
}

func (p *PackageListQueryParameters) SupportedQueryParameters() []string {
	return []string{"app_guids", "order_by", "per_page"}
}

type PackageListDropletsQueryParameters struct {
	// Below parameters are ignored, but must be included to ignore as query parameters
	States  string `schema:"states"`
	PerPage string `schema:"per_page"`
}

func (p *PackageListDropletsQueryParameters) ToMessage(packageGUIDs []string) repositories.DropletListMessage {
	return repositories.DropletListMessage{
		PackageGUIDs: packageGUIDs,
	}
}

func (p *PackageListDropletsQueryParameters) SupportedQueryParameters() []string {
	return []string{"states", "per_page"}
}
