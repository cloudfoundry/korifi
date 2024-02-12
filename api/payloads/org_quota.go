package payloads

import (
	"net/url"

	"code.cloudfoundry.org/korifi/api/payloads/parse"
	"code.cloudfoundry.org/korifi/api/repositories"
	"github.com/jellydator/validation"
)

type OrgQuotaCreate struct {
	Name      string   `json:"name"`
	Suspended bool     `json:"suspended"`
	Metadata  Metadata `json:"metadata"`
}

func (p OrgQuotaCreate) Validate() error {
	return validation.ValidateStruct(&p,
		validation.Field(&p.Name, validation.Required),
	)
}

func (p OrgQuotaCreate) ToMessage() repositories.CreateOrgMessage {
	return repositories.CreateOrgMessage{
		Name:        p.Name,
		Suspended:   p.Suspended,
		Labels:      p.Metadata.Labels,
		Annotations: p.Metadata.Annotations,
	}
}

type OrgQuotaPatch struct {
	Metadata MetadataPatch `json:"metadata"`
}

func (p OrgQuotaPatch) Validate() error {
	return validation.ValidateStruct(&p,
		validation.Field(&p.Metadata),
	)
}

func (p OrgQuotaPatch) ToMessage(orgGUID string) repositories.PatchOrgMetadataMessage {
	return repositories.PatchOrgMetadataMessage{
		GUID: orgGUID,
		MetadataPatch: repositories.MetadataPatch{
			Labels:      p.Metadata.Labels,
			Annotations: p.Metadata.Annotations,
		},
	}
}

type OrgQuotaList struct {
	Names string
}

func (d *OrgQuotaList) ToMessage() repositories.ListOrgQuotasMessage {
	return repositories.ListOrgQuotasMessage{
		Names: parse.ArrayParam(d.Names),
	}
}

func (d *OrgQuotaList) SupportedKeys() []string {
	return []string{"guids", "names", "organization_guids", "order_by", "per_page", "page"}
}

func (d *OrgQuotaList) DecodeFromURLValues(values url.Values) error {
	d.Names = values.Get("names")
	return nil
}
