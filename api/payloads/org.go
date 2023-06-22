package payloads

import (
	"net/url"

	"code.cloudfoundry.org/korifi/api/payloads/parse"
	"code.cloudfoundry.org/korifi/api/repositories"
	"github.com/jellydator/validation"
)

type OrgCreate struct {
	Name      string   `json:"name"`
	Suspended bool     `json:"suspended"`
	Metadata  Metadata `json:"metadata"`
}

func (p OrgCreate) Validate() error {
	return validation.ValidateStruct(&p,
		validation.Field(&p.Name, validation.Required),
	)
}

func (p OrgCreate) ToMessage() repositories.CreateOrgMessage {
	return repositories.CreateOrgMessage{
		Name:        p.Name,
		Suspended:   p.Suspended,
		Labels:      p.Metadata.Labels,
		Annotations: p.Metadata.Annotations,
	}
}

type OrgPatch struct {
	Metadata MetadataPatch `json:"metadata"`
}

func (p OrgPatch) Validate() error {
	return validation.ValidateStruct(&p,
		validation.Field(&p.Metadata),
	)
}

func (p OrgPatch) ToMessage(orgGUID string) repositories.PatchOrgMetadataMessage {
	return repositories.PatchOrgMetadataMessage{
		GUID: orgGUID,
		MetadataPatch: repositories.MetadataPatch{
			Labels:      p.Metadata.Labels,
			Annotations: p.Metadata.Annotations,
		},
	}
}

type OrgList struct {
	Names string
}

func (d *OrgList) ToMessage() repositories.ListOrgsMessage {
	return repositories.ListOrgsMessage{
		Names: parse.ArrayParam(d.Names),
	}
}

func (d *OrgList) SupportedKeys() []string {
	return []string{"names", "order_by", "per_page", "page"}
}

func (d *OrgList) DecodeFromURLValues(values url.Values) error {
	d.Names = values.Get("names")
	return nil
}
