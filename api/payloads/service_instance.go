package payloads

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"

	"code.cloudfoundry.org/korifi/api/payloads/parse"
	"code.cloudfoundry.org/korifi/api/payloads/validation"
	"code.cloudfoundry.org/korifi/api/repositories"
	jellidation "github.com/jellydator/validation"
)

type ServiceInstanceCreate struct {
	Name          string                        `json:"name"`
	Type          string                        `json:"type"`
	Tags          []string                      `json:"tags"`
	Credentials   map[string]string             `json:"credentials"`
	Relationships *ServiceInstanceRelationships `json:"relationships"`
	Metadata      Metadata                      `json:"metadata"`
	Parameters    map[string]any                `json:"parameters"`
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
		jellidation.Field(&c.Type, jellidation.Required, validation.OneOf("user-provided", "managed")),
		jellidation.Field(&c.Tags, jellidation.By(validateTagLength)),
		jellidation.Field(&c.Relationships, jellidation.NotNil),
		jellidation.Field(&c.Metadata),
	)
}

func (p ServiceInstanceCreate) ToServiceInstanceCreateMessage() repositories.CreateServiceInstanceMessage {
	message := repositories.CreateServiceInstanceMessage{
		Name:        p.Name,
		SpaceGUID:   p.Relationships.Space.Data.GUID,
		Credentials: p.Credentials,
		Type:        p.Type,
		Tags:        p.Tags,
		Labels:      p.Metadata.Labels,
		Annotations: p.Metadata.Annotations,
		Parameters:  p.Parameters,
	}

	if p.Relationships.ServicePlan != nil {
		message.ServicePlanGUID = p.Relationships.ServicePlan.Data.GUID
	}

	return message
}

type ServiceInstanceRelationships struct {
	Space       *Relationship `json:"space"`
	ServicePlan *Relationship `json:"service_plan"`
}

func (r ServiceInstanceRelationships) Validate() error {
	return jellidation.ValidateStruct(&r,
		jellidation.Field(&r.Space, jellidation.NotNil),
	)
}

type ServiceInstancePatch struct {
	Name        *string            `json:"name,omitempty"`
	Tags        *[]string          `json:"tags,omitempty"`
	Credentials *map[string]string `json:"credentials,omitempty"`
	Metadata    MetadataPatch      `json:"metadata"`
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
		patch.Credentials = &map[string]string{}
	}

	*p = ServiceInstancePatch(patch)

	return nil
}

type ServiceInstanceList struct {
	Names         string
	GUIDs         string
	SpaceGUIDs    string
	OrderBy       string
	LabelSelector string
}

func (l ServiceInstanceList) Validate() error {
	return jellidation.ValidateStruct(&l,
		jellidation.Field(&l.OrderBy, validation.OneOfOrderBy("created_at", "name", "updated_at")),
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
	return []string{"names", "space_guids", "guids", "order_by", "per_page", "page", "label_selector"}
}

func (l *ServiceInstanceList) IgnoredKeys() []*regexp.Regexp {
	return []*regexp.Regexp{regexp.MustCompile(`fields\[.+\]`)}
}

func (l *ServiceInstanceList) DecodeFromURLValues(values url.Values) error {
	l.Names = values.Get("names")
	l.SpaceGUIDs = values.Get("space_guids")
	l.GUIDs = values.Get("guids")
	l.OrderBy = values.Get("order_by")
	l.LabelSelector = values.Get("label_selector")
	return nil
}
