package payloads

import (
	"net/url"

	"code.cloudfoundry.org/korifi/api/payloads/parse"
	"code.cloudfoundry.org/korifi/api/repositories"
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"github.com/jellydator/validation"
)

type SpaceQuotaCreate struct {
	Name      string   `json:"name"`
	Suspended bool     `json:"suspended"`
	Metadata  Metadata `json:"metadata"`
}

type SpaceQuotaPayload struct {
	korifiv1alpha1.SpaceQuota
	Relationships korifiv1alpha1.SpaceQuotaRelationships
}

func (p SpaceQuotaPayload) Validate() error {
	return validation.ValidateStruct(&p,
		validation.Field(&p.Name, validation.Required),
		validation.Field(&p.Relationships, validation.Required),
	)
}

func (p SpaceQuotaCreate) Validate() error {
	return validation.ValidateStruct(&p,
		validation.Field(&p.Name, validation.Required),
	)
}

type SpaceQuotaPatch struct {
	Metadata MetadataPatch `json:"metadata"`
}

func (p SpaceQuotaPatch) Validate() error {
	return validation.ValidateStruct(&p,
		validation.Field(&p.Metadata),
	)
}

func (p SpaceQuotaPatch) ToMessage(orgGUID string) repositories.PatchSpaceMetadataMessage {
	return repositories.PatchSpaceMetadataMessage{
		GUID: orgGUID,
		MetadataPatch: repositories.MetadataPatch{
			Labels:      p.Metadata.Labels,
			Annotations: p.Metadata.Annotations,
		},
	}
}

type SpaceQuotaList struct {
	Names string
}

func (d *SpaceQuotaList) ToMessage() repositories.ListSpaceQuotasMessage {
	return repositories.ListSpaceQuotasMessage{
		Names: parse.ArrayParam(d.Names),
	}
}

func (d *SpaceQuotaList) SupportedKeys() []string {
	return []string{"guids", "names", "organization_guids", "space_guids", "order_by", "per_page", "page"}
}

func (d *SpaceQuotaList) DecodeFromURLValues(values url.Values) error {
	d.Names = values.Get("names")
	return nil
}
