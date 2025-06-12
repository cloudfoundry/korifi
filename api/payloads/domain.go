package payloads

import (
	"errors"
	"net/url"

	"code.cloudfoundry.org/korifi/api/payloads/parse"
	"code.cloudfoundry.org/korifi/api/payloads/validation"
	"code.cloudfoundry.org/korifi/api/repositories"
	jellidation "github.com/jellydator/validation"
)

type DomainCreate struct {
	Name          string                  `json:"name"`
	Internal      bool                    `json:"internal"`
	Metadata      Metadata                `json:"metadata"`
	Relationships map[string]Relationship `json:"relationships"`
}

func (c DomainCreate) Validate() error {
	return jellidation.ValidateStruct(&c,
		jellidation.Field(&c.Name, validation.StrictlyRequired),
		jellidation.Field(&c.Metadata),
		jellidation.Field(&c.Relationships),
	)
}

func (c *DomainCreate) ToMessage() (repositories.CreateDomainMessage, error) {
	if c.Internal {
		return repositories.CreateDomainMessage{}, errors.New("internal domains are not supported")
	}

	if len(c.Relationships) > 0 {
		return repositories.CreateDomainMessage{}, errors.New("private domains are not supported")
	}

	return repositories.CreateDomainMessage{
		Name: c.Name,
		Metadata: repositories.Metadata{
			Labels:      c.Metadata.Labels,
			Annotations: c.Metadata.Annotations,
		},
	}, nil
}

type DomainUpdate struct {
	Metadata MetadataPatch `json:"metadata"`
}

func (c *DomainUpdate) ToMessage(domainGUID string) repositories.UpdateDomainMessage {
	return repositories.UpdateDomainMessage{
		GUID: domainGUID,
		MetadataPatch: repositories.MetadataPatch{
			Labels:      c.Metadata.Labels,
			Annotations: c.Metadata.Annotations,
		},
	}
}

func (c DomainUpdate) Validate() error {
	return jellidation.ValidateStruct(&c,
		jellidation.Field(&c.Metadata),
	)
}

type DomainList struct {
	Names      string
	OrderBy    string
	Pagination Pagination
}

func (d *DomainList) ToMessage() repositories.ListDomainsMessage {
	return repositories.ListDomainsMessage{
		Names:      parse.ArrayParam(d.Names),
		OrderBy:    d.OrderBy,
		Pagination: d.Pagination.ToMessage(DefaultPageSize),
	}
}

func (d *DomainList) SupportedKeys() []string {
	return []string{"names", "order_by", "per_page", "page"}
}

func (d *DomainList) DecodeFromURLValues(values url.Values) error {
	d.Names = values.Get("names")
	d.OrderBy = values.Get("order_by")
	return d.Pagination.DecodeFromURLValues(values)
}

func (a DomainList) Validate() error {
	return jellidation.ValidateStruct(&a,
		jellidation.Field(&a.OrderBy, validation.OneOfOrderBy("created_at", "updated_at")),
		jellidation.Field(&a.Pagination),
	)
}
