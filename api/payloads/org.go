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
	Name     *string       `json:"name"`
	Metadata MetadataPatch `json:"metadata"`
}

func (p OrgPatch) Validate() error {
	return validation.ValidateStruct(&p,
		validation.Field(&p.Metadata),
		validation.Field(&p.Name, validation.NilOrNotEmpty),
	)
}

func (p OrgPatch) ToMessage(orgGUID string) repositories.PatchOrgMessage {
	return repositories.PatchOrgMessage{
		GUID: orgGUID,
		Name: p.Name,
		MetadataPatch: repositories.MetadataPatch{
			Labels:      p.Metadata.Labels,
			Annotations: p.Metadata.Annotations,
		},
	}
}

type OrgList struct {
	Names      string
	Pagination Pagination
}

func (l *OrgList) ToMessage() repositories.ListOrgsMessage {
	return repositories.ListOrgsMessage{
		Names:      parse.ArrayParam(l.Names),
		Pagination: l.Pagination.ToMessage(DefaultPageSize),
	}
}

func (l *OrgList) SupportedKeys() []string {
	return []string{"names", "order_by", "per_page", "page"}
}

func (l *OrgList) DecodeFromURLValues(values url.Values) error {
	l.Names = values.Get("names")
	return l.Pagination.DecodeFromURLValues(values)
}

func (l OrgList) Validate() error {
	return validation.ValidateStruct(&l,
		validation.Field(&l.Pagination),
	)
}
