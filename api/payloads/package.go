package payloads

import (
	"net/url"
	"strings"

	"code.cloudfoundry.org/korifi/api/repositories"
)

type PackageCreate struct {
	Type          string                `json:"type" validate:"required,oneof='bits'"`
	Relationships *PackageRelationships `json:"relationships" validate:"required"`
	Metadata      Metadata              `json:"metadata"`
}

type PackageRelationships struct {
	App *Relationship `json:"app" validate:"required"`
}

func (m PackageCreate) ToMessage(record repositories.AppRecord) repositories.CreatePackageMessage {
	return repositories.CreatePackageMessage{
		Type:      m.Type,
		AppGUID:   record.GUID,
		SpaceGUID: record.SpaceGUID,
		Metadata: repositories.Metadata{
			Annotations: m.Metadata.Annotations,
			Labels:      m.Metadata.Labels,
		},
	}
}

type PackageUpdate struct {
	Metadata MetadataPatch `json:"metadata"`
}

func (u *PackageUpdate) ToMessage(packageGUID string) repositories.UpdatePackageMessage {
	return repositories.UpdatePackageMessage{
		GUID: packageGUID,
		MetadataPatch: repositories.MetadataPatch{
			Annotations: u.Metadata.Annotations,
			Labels:      u.Metadata.Labels,
		},
	}
}

type PackageListQueryParameters struct {
	AppGUIDs string
	States   string
	OrderBy  string
}

func (p *PackageListQueryParameters) ToMessage() repositories.ListPackagesMessage {
	return repositories.ListPackagesMessage{
		AppGUIDs:        ParseArrayParam(p.AppGUIDs),
		States:          ParseArrayParam(p.States),
		SortBy:          strings.TrimPrefix(p.OrderBy, "-"),
		DescendingOrder: strings.HasPrefix(p.OrderBy, "-"),
	}
}

func (p *PackageListQueryParameters) SupportedKeys() []string {
	return []string{"app_guids", "order_by", "per_page", "states"}
}

func (p *PackageListQueryParameters) DecodeFromURLValues(values url.Values) error {
	p.AppGUIDs = values.Get("app_guids")
	p.OrderBy = values.Get("order_by")
	p.States = values.Get("states")
	return nil
}

type PackageListDropletsQueryParameters struct{}

func (p *PackageListDropletsQueryParameters) ToMessage(packageGUIDs []string) repositories.ListDropletsMessage {
	return repositories.ListDropletsMessage{
		PackageGUIDs: packageGUIDs,
	}
}

func (p *PackageListDropletsQueryParameters) SupportedKeys() []string {
	return []string{"states", "per_page"}
}

func (p *PackageListDropletsQueryParameters) DecodeFromURLValues(values url.Values) error {
	return nil
}
