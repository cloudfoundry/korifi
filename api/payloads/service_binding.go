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
		ServiceInstanceGUID: p.Relationships.ServiceInstance.Data.GUID,
		AppGUID:             p.Relationships.App.Data.GUID,
		SpaceGUID:           spaceGUID,
	}
}

type ServiceBindingList struct {
	AppGUIDs             string
	ServiceInstanceGUIDs string
	Include              string
}

func (l *ServiceBindingList) ToMessage() repositories.ListServiceBindingsMessage {
	return repositories.ListServiceBindingsMessage{
		ServiceInstanceGUIDs: ParseArrayParam(l.ServiceInstanceGUIDs),
		AppGUIDs:             ParseArrayParam(l.AppGUIDs),
	}
}

func (l *ServiceBindingList) SupportedKeys() []string {
	return []string{"app_guids", "service_instance_guids", "include", "type"}
}

func (l *ServiceBindingList) DecodeFromURLValues(values url.Values) error {
	l.AppGUIDs = values.Get("app_guids")
	l.ServiceInstanceGUIDs = values.Get("service_instance_guids")
	l.Include = values.Get("include")
	return nil
}
