package payloads

import "code.cloudfoundry.org/korifi/api/repositories"

type DropletUpdate struct {
	Metadata MetadataPatch `json:"metadata"`
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
