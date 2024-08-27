package payloads

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"code.cloudfoundry.org/korifi/api/payloads/params"
	"code.cloudfoundry.org/korifi/api/payloads/parse"
	"code.cloudfoundry.org/korifi/api/payloads/validation"
	"code.cloudfoundry.org/korifi/api/repositories"
	jellidation "github.com/jellydator/validation"
)

type ServiceInstanceCreate struct {
	Name          string                        `json:"name"`
	Type          string                        `json:"type"`
	Tags          []string                      `json:"tags"`
	Credentials   map[string]any                `json:"credentials"`
	Relationships *ServiceInstanceRelationships `json:"relationships"`
	Metadata      Metadata                      `json:"metadata"`
}

const maxTagsLength = 2048

func validateTagLength(tags any) error {
	tagSlice, ok := tags.([]string)
	if !ok {
		return errors.New("wrong input")
	}

	l := 0
	for _, t := range tagSlice {
		l += len(t)
		if l >= maxTagsLength {
			return fmt.Errorf("combined length of tags cannot exceed %d", maxTagsLength)
		}
	}

	return nil
}

func (c ServiceInstanceCreate) Validate() error {
	return jellidation.ValidateStruct(&c,
		jellidation.Field(&c.Name, jellidation.Required),
		jellidation.Field(&c.Type, jellidation.Required, validation.OneOf("user-provided")),
		jellidation.Field(&c.Tags, jellidation.By(validateTagLength)),
		jellidation.Field(&c.Relationships, jellidation.NotNil),
		jellidation.Field(&c.Metadata),
	)
}

func (p ServiceInstanceCreate) ToServiceInstanceCreateMessage() repositories.CreateServiceInstanceMessage {
	return repositories.CreateServiceInstanceMessage{
		Name:        p.Name,
		SpaceGUID:   p.Relationships.Space.Data.GUID,
		Credentials: p.Credentials,
		Type:        p.Type,
		Tags:        p.Tags,
		Labels:      p.Metadata.Labels,
		Annotations: p.Metadata.Annotations,
	}
}

type ServiceInstanceRelationships struct {
	Space *Relationship `json:"space"`
}

func (r ServiceInstanceRelationships) Validate() error {
	return jellidation.ValidateStruct(&r,
		jellidation.Field(&r.Space, jellidation.NotNil),
	)
}

type ServiceInstancePatch struct {
	Name        *string         `json:"name,omitempty"`
	Tags        *[]string       `json:"tags,omitempty"`
	Credentials *map[string]any `json:"credentials,omitempty"`
	Metadata    MetadataPatch   `json:"metadata"`
}

func (p ServiceInstancePatch) Validate() error {
	return jellidation.ValidateStruct(&p,
		jellidation.Field(&p.Metadata),
	)
}

func (p ServiceInstancePatch) ToServiceInstancePatchMessage(spaceGUID, appGUID string) repositories.PatchServiceInstanceMessage {
	return repositories.PatchServiceInstanceMessage{
		SpaceGUID:   spaceGUID,
		GUID:        appGUID,
		Name:        p.Name,
		Credentials: p.Credentials,
		Tags:        p.Tags,
		MetadataPatch: repositories.MetadataPatch{
			Labels:      p.Metadata.Labels,
			Annotations: p.Metadata.Annotations,
		},
	}
}

func (p *ServiceInstancePatch) UnmarshalJSON(data []byte) error {
	type alias ServiceInstancePatch

	var patch alias
	err := json.Unmarshal(data, &patch)
	if err != nil {
		return err
	}

	var patchMap map[string]any
	err = json.Unmarshal(data, &patchMap)
	if err != nil {
		return err
	}

	if v, ok := patchMap["tags"]; ok && v == nil {
		patch.Tags = &[]string{}
	}

	if v, ok := patchMap["credentials"]; ok && v == nil {
		patch.Credentials = &map[string]any{}
	}

	*p = ServiceInstancePatch(patch)

	return nil
}

type ServiceInstanceList struct {
	Names                string
	GUIDs                string
	SpaceGUIDs           string
	OrderBy              string
	LabelSelector        string
	IncludeResourceRules []params.IncludeResourceRule
}

func (l ServiceInstanceList) Validate() error {
	return jellidation.ValidateStruct(&l,
		jellidation.Field(&l.OrderBy, validation.OneOfOrderBy("created_at", "name", "updated_at")),
		jellidation.Field(&l.IncludeResourceRules, jellidation.Each(jellidation.By(func(value any) error {
			rule, ok := value.(params.IncludeResourceRule)
			if !ok {
				return fmt.Errorf("%T is not supported, IncludeResourceRule is expected", value)
			}

			if strings.Join(rule.RelationshipPath, ".") != "service_plan.service_offering.service_broker" {
				return jellidation.NewError("invalid_fields_param", "must be fields[service_plan.service_offering.service_broker]")
			}

			return jellidation.Each(validation.OneOf("guid", "name")).Validate(rule.Fields)
		}))),
	)
}

func (l *ServiceInstanceList) ToMessage() repositories.ListServiceInstanceMessage {
	return repositories.ListServiceInstanceMessage{
		Names:         parse.ArrayParam(l.Names),
		SpaceGUIDs:    parse.ArrayParam(l.SpaceGUIDs),
		GUIDs:         parse.ArrayParam(l.GUIDs),
		LabelSelector: l.LabelSelector,
	}
}

func (l *ServiceInstanceList) SupportedKeys() []string {
	return []string{"names", "space_guids", "guids", "order_by", "label_selector", "fields[service_plan.service_offering.service_broker]"}
}

func (l *ServiceInstanceList) IgnoredKeys() []*regexp.Regexp {
	return []*regexp.Regexp{
		regexp.MustCompile("page"),
		regexp.MustCompile("per_page"),
	}
}

func (l *ServiceInstanceList) DecodeFromURLValues(values url.Values) error {
	l.Names = values.Get("names")
	l.SpaceGUIDs = values.Get("space_guids")
	l.GUIDs = values.Get("guids")
	l.OrderBy = values.Get("order_by")
	l.LabelSelector = values.Get("label_selector")
	l.IncludeResourceRules = append(l.IncludeResourceRules, params.ParseFields(values)...)
	return nil
}
