package payloads

import (
	"code.cloudfoundry.org/korifi/api/repositories"
	"github.com/jellydator/validation"
)

type DropletUpdate struct {
	Metadata MetadataPatch `json:"metadata"`
}

func (d DropletUpdate) Validate() error {
	return validation.ValidateStruct(&d,
		validation.Field(&d.Metadata),
	)
}

func (c *DropletUpdate) ToMessage(dropletGUID string) repositories.UpdateDropletMessage {
	return repositories.UpdateDropletMessage{
		GUID: dropletGUID,
		MetadataPatch: repositories.MetadataPatch{
			Labels:      c.Metadata.Labels,
			Annotations: c.Metadata.Annotations,
		},
	}
}
