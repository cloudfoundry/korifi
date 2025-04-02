package payloads

import (
	"fmt"
	"net/url"

	"code.cloudfoundry.org/korifi/api/payloads/parse"
	"code.cloudfoundry.org/korifi/api/repositories"
	"github.com/jellydator/validation"
)

type SpaceCreate struct {
	Name          string              `json:"name"`
	Relationships *SpaceRelationships `json:"relationships"`
	Metadata      Metadata            `json:"metadata"`
}

func (c SpaceCreate) Validate() error {
	return validation.ValidateStruct(&c,
		validation.Field(&c.Name, validation.Required),
		validation.Field(&c.Relationships, validation.NotNil),
		validation.Field(&c.Metadata),
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
	return validation.ValidateStruct(&r,
		validation.Field(&r.Org, validation.NotNil),
	)
}

type SpacePatch struct {
	Metadata MetadataPatch `json:"metadata"`
}

func (p SpacePatch) Validate() error {
	return validation.ValidateStruct(&p,
		validation.Field(&p.Metadata),
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

type SpaceList struct {
	Names             string
	GUIDs             string
	OrganizationGUIDs string
}

func (l *SpaceList) ToMessage() repositories.ListSpacesMessage {
	return repositories.ListSpacesMessage{
		Names:             parse.ArrayParam(l.Names),
		GUIDs:             parse.ArrayParam(l.GUIDs),
		OrganizationGUIDs: parse.ArrayParam(l.OrganizationGUIDs),
	}
}

func (l *SpaceList) SupportedKeys() []string {
	return []string{"names", "guids", "organization_guids", "order_by", "per_page", "page"}
}

func (l *SpaceList) DecodeFromURLValues(values url.Values) error {
	l.Names = values.Get("names")
	l.GUIDs = values.Get("guids")
	l.OrganizationGUIDs = values.Get("organization_guids")
	return nil
}

type SpaceDeleteRoutes struct {
	Unmapped string `json:"unmapped"`
}

func (d *SpaceDeleteRoutes) SupportedKeys() []string {
	return []string{"unmapped"}
}

func (d SpaceDeleteRoutes) Validate() error {
	return validation.ValidateStruct(&d,
		validation.Field(&d.Unmapped, validation.Required, validation.By(func(value any) error {
			unmappedStr, _ := value.(string)

			switch unmappedStr {
			case "true":
				return nil
			case "false":
				return fmt.Errorf("mass delete not supported for mapped routes. Use 'unmapped=true' parameter to delete all unmapped routes")
			default:
				return fmt.Errorf("must be a boolean")
			}
		})),
	)
}

func (d *SpaceDeleteRoutes) DecodeFromURLValues(values url.Values) error {
	d.Unmapped = values.Get("unmapped")
	return nil
}
