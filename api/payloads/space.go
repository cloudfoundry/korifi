package payloads

import (
	"fmt"
	"net/url"

	"code.cloudfoundry.org/korifi/api/payloads/params"
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
	Name     *string       `json:"name"`
	Metadata MetadataPatch `json:"metadata"`
}

func (p SpacePatch) Validate() error {
	return jellidation.ValidateStruct(&p,
		jellidation.Field(&p.Metadata),
		jellidation.Field(&p.Name, jellidation.NilOrNotEmpty),
	)
}

func (p SpacePatch) ToMessage(spaceGUID, orgGUID string) repositories.PatchSpaceMessage {
	return repositories.PatchSpaceMessage{
		GUID:    spaceGUID,
		OrgGUID: orgGUID,
		Name:    p.Name,
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
	return jellidation.ValidateStruct(&g,
		jellidation.Field(&g.IncludeResourceRules, jellidation.Each(jellidation.By(func(value any) error {
			rule, ok := value.(params.IncludeResourceRule)
			if !ok {
				return fmt.Errorf("%T is not supported, IncludeResourceRule is expected", value)
			}

			if len(rule.RelationshipPath) > 0 && rule.RelationshipPath[0] != "organization" {
				return jellidation.NewError("invalid_fields_param", "must be organization")
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
	if includeVal != "" {
		s.IncludeResourceRules = []params.IncludeResourceRule{
			{
				RelationshipPath: []string{includeVal},
			},
		}
	}

	return nil
}

type SpaceList struct {
	Names                string
	GUIDs                string
	OrganizationGUIDs    string
	IncludeResourceRules []params.IncludeResourceRule
	Pagination           Pagination
}

func (s SpaceList) Validate() error {
	return jellidation.ValidateStruct(&s,
		jellidation.Field(&s.IncludeResourceRules, jellidation.Each(jellidation.By(func(value any) error {
			rule, ok := value.(params.IncludeResourceRule)
			if !ok {
				return fmt.Errorf("%T is not supported, IncludeResourceRule is expected", value)
			}

			if len(rule.RelationshipPath) > 0 && rule.RelationshipPath[0] != "organization" {
				return jellidation.NewError("invalid_fields_param", "must be organization")
			}

			return nil
		}))),
		jellidation.Field(&s.Pagination),
	)
}

func (l *SpaceList) ToMessage() repositories.ListSpacesMessage {
	return repositories.ListSpacesMessage{
		Names:             parse.ArrayParam(l.Names),
		GUIDs:             parse.ArrayParam(l.GUIDs),
		OrganizationGUIDs: parse.ArrayParam(l.OrganizationGUIDs),
		Pagination:        l.Pagination.ToMessage(DefaultPageSize),
	}
}

func (l *SpaceList) SupportedKeys() []string {
	return []string{"names", "guids", "organization_guids", "order_by", "per_page", "page", "include"}
}

func (l *SpaceList) DecodeFromURLValues(values url.Values) error {
	l.Names = values.Get("names")
	l.GUIDs = values.Get("guids")
	l.OrganizationGUIDs = values.Get("organization_guids")

	includeVal := values.Get("include")
	if includeVal != "" {
		l.IncludeResourceRules = []params.IncludeResourceRule{
			{
				RelationshipPath: []string{includeVal},
			},
		}
	}

	return l.Pagination.DecodeFromURLValues(values)
}

type SpaceDeleteRoutes struct {
	Unmapped string `json:"unmapped"`
}

func (d *SpaceDeleteRoutes) SupportedKeys() []string {
	return []string{"unmapped"}
}

func (d SpaceDeleteRoutes) Validate() error {
	return jellidation.ValidateStruct(&d,
		jellidation.Field(&d.Unmapped,
			jellidation.Required,
			jellidation.StringIn(true, "true", "false").Error("must be a boolean"),
			jellidation.StringIn(true, "true").Error("mass delete not supported for mapped routes. Use 'unmapped=true' parameter to delete all unmapped routes"),
		),
	)
}

func (d *SpaceDeleteRoutes) DecodeFromURLValues(values url.Values) error {
	d.Unmapped = values.Get("unmapped")
	return nil
}
