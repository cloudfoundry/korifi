package payloads

import (
	"errors"
	"net/url"

	"code.cloudfoundry.org/korifi/api/repositories"
)

type DomainCreate struct {
	Name          string                  `json:"name" validate:"required"`
	Internal      bool                    `json:"internal"`
	Metadata      Metadata                `json:"metadata"`
	Relationships map[string]Relationship `json:"relationships"`
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

type DomainList struct {
	Names string
}

func (d *DomainList) ToMessage() repositories.ListDomainsMessage {
	return repositories.ListDomainsMessage{
		Names: ParseArrayParam(d.Names),
	}
}

func (d *DomainList) SupportedKeys() []string {
	return []string{"names", "page"}
}

func (d *DomainList) DecodeFromURLValues(values url.Values) error {
	d.Names = values.Get("names")
	return nil
}
