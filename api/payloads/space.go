package payloads

import "code.cloudfoundry.org/korifi/api/repositories"

type SpaceCreate struct {
	Name          string             `json:"name" validate:"required"`
	Relationships SpaceRelationships `json:"relationships" validate:"required"`
	Metadata      Metadata           `json:"metadata"`
}

type SpaceRelationships struct {
	Org Relationship `json:"organization" validate:"required"`
}

func (p SpaceCreate) ToMessage() repositories.CreateSpaceMessage {
	return repositories.CreateSpaceMessage{
		Name:             p.Name,
		OrganizationGUID: p.Relationships.Org.Data.GUID,
	}
}

type SpacePatch struct {
	Metadata MetadataPatch `json:"metadata"`
}

func (p SpacePatch) ToMessage(spaceGUID, orgGUID string) repositories.PatchSpaceMetadataMessage {
	return repositories.PatchSpaceMetadataMessage{
		GUID:    spaceGUID,
		OrgGUID: orgGUID,
		MetadataPatch: repositories.MetadataPatch{
			Labels:      p.Metadata.Labels,
			Annotations: p.Metadata.Annotations,
		},
	}
}
