package payloads

import (
	"encoding/json"
	"net/url"

	"code.cloudfoundry.org/korifi/api/repositories"
)

type ServiceInstanceCreate struct {
	Name          string                       `json:"name" validate:"required"`
	Type          string                       `json:"type" validate:"required,oneof=user-provided managed"`
	Tags          []string                     `json:"tags" validate:"serviceinstancetaglength"`
	Credentials   map[string]string            `json:"credentials"`
	Relationships ServiceInstanceRelationships `json:"relationships" validate:"required"`
	Metadata      Metadata                     `json:"metadata"`
}

type ServiceInstanceRelationships struct {
	Space       Relationship `json:"space" validate:"required"`
	ServicePlan Relationship `json:"service_plan"`
}

func (p ServiceInstanceCreate) ToServiceInstanceCreateMessage() repositories.CreateServiceInstanceMessage {
	return repositories.CreateServiceInstanceMessage{
		Name:            p.Name,
		SpaceGUID:       p.Relationships.Space.Data.GUID,
		ServicePlanGUID: p.Relationships.ServicePlan.Data.GUID,
		Credentials:     p.Credentials,
		Type:            p.Type,
		Tags:            p.Tags,
		Labels:          p.Metadata.Labels,
		Annotations:     p.Metadata.Annotations,
	}
}

type ServiceInstancePatch struct {
	Name        *string            `json:"name,omitempty"`
	Tags        *[]string          `json:"tags,omitempty"`
	Credentials *map[string]string `json:"credentials,omitempty"`
	Metadata    MetadataPatch      `json:"metadata"`
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
	Names      string
	SpaceGuids string
	OrderBy    string
}

func (l *ServiceInstanceList) ToMessage() repositories.ListServiceInstanceMessage {
	return repositories.ListServiceInstanceMessage{
		Names:      ParseArrayParam(l.Names),
		SpaceGuids: ParseArrayParam(l.SpaceGuids),
	}
}

func (l *ServiceInstanceList) SupportedKeys() []string {
	return []string{"names", "space_guids", "fields", "order_by", "per_page"}
}

func (l *ServiceInstanceList) DecodeFromURLValues(values url.Values) error {
	l.Names = values.Get("names")
	l.SpaceGuids = values.Get("space_guids")
	l.OrderBy = values.Get("order_by")
	return nil
}
