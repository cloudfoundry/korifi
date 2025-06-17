package payloads

import (
	"net/url"

	"code.cloudfoundry.org/korifi/api/payloads/parse"
	"code.cloudfoundry.org/korifi/api/payloads/validation"
	"code.cloudfoundry.org/korifi/api/repositories"

	jellidation "github.com/jellydator/validation"
)

type PackageCreate struct {
	Type          string                `json:"type"`
	Relationships *PackageRelationships `json:"relationships"`
	Metadata      Metadata              `json:"metadata"`
	Data          *PackageData          `json:"data"`
}

func (c PackageCreate) Validate() error {
	return jellidation.ValidateStruct(&c,
		jellidation.Field(&c.Type, validation.OneOf("bits", "docker"), jellidation.Required),
		jellidation.Field(&c.Relationships, jellidation.NotNil),
		jellidation.Field(&c.Metadata),
		jellidation.Field(&c.Data, jellidation.When(c.Type == "docker", jellidation.Required).Else(jellidation.Nil)),
	)
}

func (c PackageCreate) ToMessage(record repositories.AppRecord) repositories.CreatePackageMessage {
	message := repositories.CreatePackageMessage{
		Type:      c.Type,
		AppGUID:   record.GUID,
		SpaceGUID: record.SpaceGUID,
		Metadata: repositories.Metadata{
			Annotations: c.Metadata.Annotations,
			Labels:      c.Metadata.Labels,
		},
	}

	if c.Type == "docker" {
		message.Data = &repositories.PackageData{
			Image:    c.Data.Image,
			Username: c.Data.Username,
			Password: c.Data.Password,
		}
	}

	return message
}

type PackageData struct {
	Image    string  `json:"image"`
	Username *string `json:"username"`
	Password *string `json:"password"`
}

func (d PackageData) Validate() error {
	return jellidation.ValidateStruct(&d,
		jellidation.Field(&d.Image, jellidation.Required),
	)
}

type PackageRelationships struct {
	App *Relationship `json:"app"`
}

func (r PackageRelationships) Validate() error {
	return jellidation.ValidateStruct(&r,
		jellidation.Field(&r.App, jellidation.NotNil))
}

type PackageUpdate struct {
	Metadata MetadataPatch `json:"metadata"`
}

func (p PackageUpdate) Validate() error {
	return jellidation.ValidateStruct(&p,
		jellidation.Field(&p.Metadata),
	)
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
	GUIDs      string
	AppGUIDs   string
	States     string
	OrderBy    string
	Pagination Pagination
}

func (p *PackageList) ToMessage() repositories.ListPackagesMessage {
	return repositories.ListPackagesMessage{
		GUIDs:      parse.ArrayParam(p.GUIDs),
		AppGUIDs:   parse.ArrayParam(p.AppGUIDs),
		States:     parse.ArrayParam(p.States),
		OrderBy:    p.OrderBy,
		Pagination: p.Pagination.ToMessage(DefaultPageSize),
	}
}

func (p *PackageList) SupportedKeys() []string {
	return []string{"guids", "app_guids", "states", "order_by", "per_page", "page"}
}

func (p *PackageList) DecodeFromURLValues(values url.Values) error {
	p.GUIDs = values.Get("guids")
	p.AppGUIDs = values.Get("app_guids")
	p.States = values.Get("states")
	p.OrderBy = values.Get("order_by")
	return p.Pagination.DecodeFromURLValues(values)
}

func (p PackageList) Validate() error {
	return jellidation.ValidateStruct(&p,
		jellidation.Field(&p.OrderBy, validation.OneOfOrderBy("created_at", "updated_at")),
		jellidation.Field(&p.Pagination),
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
