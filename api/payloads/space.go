package payloads

import (
	"fmt"
	"net/url"

	"code.cloudfoundry.org/korifi/api/payloads/params"
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

type SpaceGet struct {
	IncludeResourceRules []params.IncludeResourceRule
}

func (g SpaceGet) Validate() error {
	return validation.ValidateStruct(&g,
		validation.Field(&g.IncludeResourceRules, validation.Each(validation.By(func(value any) error {
			rule, ok := value.(params.IncludeResourceRule)
			if !ok {
				return fmt.Errorf("%T is not supported, IncludeResourceRule is expected", value)
			}

			if rule.RelationshipPath[0] != "organization" {
				return validation.NewError("invalid_fields_param", "must be organization")
			}

			return nil
		}))),
	)
}

func (s *SpaceGet) SupportedKeys() []string {
	return []string{"include"}
}
func (s *SpaceGet) DecodeFromURLValues(values url.Values) error {
	includeVal := values.Get("include")
	s.IncludeResourceRules = []params.IncludeResourceRule{
		params.IncludeResourceRule{
			RelationshipPath: []string{includeVal},
		},
	}
	return nil
}

type SpaceList struct {
	Names                string
	GUIDs                string
	Include              string
	OrganizationGUIDs    string
	IncludeResourceRules []params.IncludeResourceRule
}

func (s SpaceList) Validate() error {
	return validation.ValidateStruct(&s,
		validation.Field(&s.Include,
			validation.Required.When(s.Include != ""),
			validation.In("organization"),
		),
		validation.Field(&s.IncludeResourceRules, validation.Each(validation.By(func(value any) error {
			rule, ok := value.(params.IncludeResourceRule)
			if !ok {
				return fmt.Errorf("%T is not supported, IncludeResourceRule is expected", value)
			}

			if rule.RelationshipPath[0] != "organization" {
				return validation.NewError("invalid_fields_param", "must be organization")
			}

			return nil
		}))),
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
