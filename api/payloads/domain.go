package payloads

import (
	"errors"
	"net/url"

	"code.cloudfoundry.org/korifi/api/payloads/parse"
	payload_validation "code.cloudfoundry.org/korifi/api/payloads/validation"
	"code.cloudfoundry.org/korifi/api/repositories"
	"github.com/jellydator/validation"
)

type DomainCreate struct {
	Name          string                  `json:"name"`
	Internal      bool                    `json:"internal"`
	Metadata      Metadata                `json:"metadata"`
	Relationships map[string]Relationship `json:"relationships"`
}

func (c DomainCreate) Validate() error {
	return validation.ValidateStruct(&c,
		validation.Field(&c.Name, payload_validation.StrictlyRequired),
		validation.Field(&c.Metadata),
		validation.Field(&c.Relationships),
	)
}

func (c *DomainCreate) ToMessage() (repositories.CreateDomainMessage, error) {
	if c.Internal {
		return repositories.CreateDomainMessage{}, errors.New("internal domains are not supported")
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
	return validation.ValidateStruct(&c,
		validation.Field(&c.Metadata),
	)
}

type DomainList struct {
	Names string
}

func (d *DomainList) ToMessage() repositories.ListDomainsMessage {
	return repositories.ListDomainsMessage{
		Names: parse.ArrayParam(d.Names),
	}
}

func (d *DomainList) SupportedKeys() []string {
	return []string{"names", "per_page", "page"}
}

func (d *DomainList) DecodeFromURLValues(values url.Values) error {
	d.Names = values.Get("names")
	return nil
}
