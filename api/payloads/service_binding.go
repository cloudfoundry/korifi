package payloads

import (
	"net/url"

	"code.cloudfoundry.org/korifi/api/repositories"
)

type ServiceBindingCreate struct {
	Relationships *ServiceBindingRelationships `json:"relationships" validate:"required"`
	Type          string                       `json:"type" validate:"oneof=app"`
	Name          *string                      `json:"name"`
}

type ServiceBindingRelationships struct {
	App             *Relationship `json:"app" validate:"required"`
	ServiceInstance *Relationship `json:"service_instance" validate:"required"`
}

func (p ServiceBindingCreate) ToMessage(spaceGUID string) repositories.CreateServiceBindingMessage {
	return repositories.CreateServiceBindingMessage{
		Name:                p.Name,
		Type:                p.Type,
		ServiceInstanceGUID: p.Relationships.ServiceInstance.Data.GUID,
		AppGUID:             p.Relationships.App.Data.GUID,
		SpaceGUID:           spaceGUID,
	}
}

type ServiceBindingList struct {
	Type                 string
	AppGUIDs             string
	ServiceInstanceGUIDs string
	Include              string
	LabelSelector        string
}

func (l *ServiceBindingList) ToMessage() repositories.ListServiceBindingsMessage {
	return repositories.ListServiceBindingsMessage{
		Type:                 l.Type,
		ServiceInstanceGUIDs: ParseArrayParam(l.ServiceInstanceGUIDs),
		AppGUIDs:             ParseArrayParam(l.AppGUIDs),
		LabelSelectors:       ParseArrayParam(l.LabelSelector),
	}
}

func (l *ServiceBindingList) SupportedKeys() []string {
	return []string{"app_guids", "service_instance_guids", "include", "type", "label_selector", "page"}
}

func (l *ServiceBindingList) DecodeFromURLValues(values url.Values) error {
	l.Type = values.Get("type")
	if l.Type != "key" {
		l.Type = "app"
	}
	l.AppGUIDs = values.Get("app_guids")
	l.ServiceInstanceGUIDs = values.Get("service_instance_guids")
	l.Include = values.Get("include")
	return nil
}

type ServiceBindingUpdate struct {
	Metadata MetadataPatch `json:"metadata"`
}

func (c *ServiceBindingUpdate) ToMessage(serviceBindingGUID string) repositories.UpdateServiceBindingMessage {
	return repositories.UpdateServiceBindingMessage{
		GUID: serviceBindingGUID,
		MetadataPatch: repositories.MetadataPatch{
			Labels:      c.Metadata.Labels,
			Annotations: c.Metadata.Annotations,
		},
	}
}
