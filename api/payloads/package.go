package payloads

import (
	"net/url"

	"code.cloudfoundry.org/korifi/api/payloads/parse"
	"code.cloudfoundry.org/korifi/api/payloads/validation"
	"code.cloudfoundry.org/korifi/api/repositories"
	jellidation "github.com/jellydator/validation"
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

type PackageList struct {
	AppGUIDs string
	States   string
	OrderBy  string
}

func (p *PackageList) ToMessage() repositories.ListPackagesMessage {
	return repositories.ListPackagesMessage{
		AppGUIDs: parse.ArrayParam(p.AppGUIDs),
		States:   parse.ArrayParam(p.States),
	}
}

func (p *PackageList) SupportedKeys() []string {
	return []string{"app_guids", "states", "order_by", "per_page", "page"}
}

func (p *PackageList) DecodeFromURLValues(values url.Values) error {
	p.AppGUIDs = values.Get("app_guids")
	p.States = values.Get("states")
	p.OrderBy = values.Get("order_by")
	return nil
}

func (p PackageList) Validate() error {
	validOrderBys := []string{"created_at", "updated_at"}
	var allowed []any
	for _, a := range validOrderBys {
		allowed = append(allowed, a, "-"+a)
	}

	return jellidation.ValidateStruct(&p,
		jellidation.Field(&p.OrderBy, validation.OneOf(allowed...)),
	)
}

type PackageListDroplets struct{}

func (p *PackageListDroplets) ToMessage(packageGUIDs []string) repositories.ListDropletsMessage {
	return repositories.ListDropletsMessage{
		PackageGUIDs: packageGUIDs,
	}
}

func (p *PackageListDroplets) SupportedKeys() []string {
	return []string{"states", "per_page", "page"}
}

func (p *PackageListDroplets) DecodeFromURLValues(values url.Values) error {
	return nil
}
