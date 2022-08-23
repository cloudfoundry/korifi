package payloads

import "code.cloudfoundry.org/korifi/api/repositories"

type OrgCreate struct {
	Name      string   `json:"name" validate:"required"`
	Suspended bool     `json:"suspended"`
	Metadata  Metadata `json:"metadata"`
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

func (p OrgPatch) ToMessage(orgGUID string) repositories.PatchOrgMetadataMessage {
	return repositories.PatchOrgMetadataMessage{
		OrgGUID:     orgGUID,
		Labels:      p.Metadata.Labels,
		Annotations: p.Metadata.Annotations,
	}
}
