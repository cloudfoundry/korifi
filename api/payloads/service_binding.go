package payloads

import (
	"code.cloudfoundry.org/cf-k8s-controllers/api/repositories"
)

type ServiceBindingCreate struct { // TODO: add validation
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

func (l *ServiceBindingList) ToMessage() repositories.ListServiceBindingsMessage {
	return repositories.ListServiceBindingsMessage{
		ServiceInstanceGUIDs: ParseArrayParam(l.ServiceInstanceGuids),
	}
}

type ServiceBindingList struct {
	ServiceInstanceGuids *string `schema:"service_instance_guids"`
}

func (l *ServiceBindingList) SupportedFilterKeys() []string {
	return []string{"service_instance_guids"}
}
