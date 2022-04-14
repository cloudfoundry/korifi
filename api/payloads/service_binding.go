package payloads

import (
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
	AppGUIDs             *string `schema:"app_guids"`
	ServiceInstanceGUIDs *string `schema:"service_instance_guids"`
	Include              *string `schema:"include" validate:"oneof=app"`
	Type                 *string `schema:"type" validate:"oneof=app"`
}

func (l *ServiceBindingList) ToMessage() repositories.ListServiceBindingsMessage {
	return repositories.ListServiceBindingsMessage{
		ServiceInstanceGUIDs: ParseArrayParam(l.ServiceInstanceGUIDs),
		AppGUIDs:             ParseArrayParam(l.AppGUIDs),
	}
}

func (l *ServiceBindingList) SupportedFilterKeys() []string {
	return []string{"app_guids, service_instance_guids, include, type"}
}
