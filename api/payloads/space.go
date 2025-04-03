package payloads

import (
	"net/url"

	"code.cloudfoundry.org/korifi/api/payloads/parse"
	"code.cloudfoundry.org/korifi/api/repositories"
	jellidation "github.com/jellydator/validation"
)

type SpaceCreate struct {
	Name          string              `json:"name"`
	Relationships *SpaceRelationships `json:"relationships"`
	Metadata      Metadata            `json:"metadata"`
}

func (c SpaceCreate) Validate() error {
	return jellidation.ValidateStruct(&c,
		jellidation.Field(&c.Name, jellidation.Required),
		jellidation.Field(&c.Relationships, jellidation.NotNil),
		jellidation.Field(&c.Metadata),
	)
}

func (p SpaceCreate) ToMessage() repositories.CreateSpaceMessage {
	return repositories.CreateSpaceMessage{
		Name:             p.Name,
		OrganizationGUID: p.Relationships.Org.Data.GUID,
	}
}

type SpaceRelationships struct {
	Org *Relationship `json:"organization"`
}

func (r SpaceRelationships) Validate() error {
	return jellidation.ValidateStruct(&r,
		jellidation.Field(&r.Org, jellidation.NotNil),
	)
}

type SpacePatch struct {
	Metadata MetadataPatch `json:"metadata"`
}

func (p SpacePatch) Validate() error {
	return jellidation.ValidateStruct(&p,
		jellidation.Field(&p.Metadata),
	)
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

type SpaceGet struct {
	Include string
}

func (s SpaceGet) Validate() error {
	return jellidation.ValidateStruct(&s,
		jellidation.Field(&s.Include,
			jellidation.Required.When(s.Include != ""),
			jellidation.In("organization"),
		),
	)
}

func (s *SpaceGet) SupportedKeys() []string {
	return []string{"include"}
}
func (s *SpaceGet) DecodeFromURLValues(values url.Values) error {
	s.Include = values.Get("include")
	return nil
}

type SpaceList struct {
	Names             string
	GUIDs             string
	Include           string
	OrganizationGUIDs string
}

func (s SpaceList) Validate() error {
	return jellidation.ValidateStruct(&s,
		jellidation.Field(&s.Include,
			jellidation.Required.When(s.Include != ""),
			jellidation.In("organization"),
		),
	)
}

func (l *SpaceList) ToMessage() repositories.ListSpacesMessage {
	return repositories.ListSpacesMessage{
		Names:             parse.ArrayParam(l.Names),
		GUIDs:             parse.ArrayParam(l.GUIDs),
		OrganizationGUIDs: parse.ArrayParam(l.OrganizationGUIDs),
	}
}

func (l *SpaceList) SupportedKeys() []string {
	return []string{"names", "guids", "organization_guids", "order_by", "per_page", "page", "include"}
}

func (l *SpaceList) DecodeFromURLValues(values url.Values) error {
	l.Names = values.Get("names")
	l.GUIDs = values.Get("guids")
	l.OrganizationGUIDs = values.Get("organization_guids")
	l.Include = values.Get("include")
	return nil
}
