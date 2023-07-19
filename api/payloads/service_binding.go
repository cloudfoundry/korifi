package payloads

import (
	"net/url"

	"code.cloudfoundry.org/korifi/api/payloads/parse"
	"code.cloudfoundry.org/korifi/api/payloads/validation"
	"code.cloudfoundry.org/korifi/api/repositories"
	jellidation "github.com/jellydator/validation"
	"k8s.io/apimachinery/pkg/labels"
)

type ServiceBindingCreate struct {
	Relationships *ServiceBindingRelationships `json:"relationships"`
	Type          string                       `json:"type"`
	Name          *string                      `json:"name"`
}

func (p ServiceBindingCreate) ToMessage(spaceGUID string) repositories.CreateServiceBindingMessage {
	return repositories.CreateServiceBindingMessage{
		Name:                p.Name,
		ServiceInstanceGUID: p.Relationships.ServiceInstance.Data.GUID,
		AppGUID:             p.Relationships.App.Data.GUID,
		SpaceGUID:           spaceGUID,
	}
}

func (p ServiceBindingCreate) Validate() error {
	return jellidation.ValidateStruct(&p,
		jellidation.Field(&p.Type, validation.OneOf("app")),
		jellidation.Field(&p.Relationships, jellidation.NotNil),
	)
}

type ServiceBindingRelationships struct {
	App             *Relationship `json:"app"`
	ServiceInstance *Relationship `json:"service_instance"`
}

func (r ServiceBindingRelationships) Validate() error {
	return jellidation.ValidateStruct(&r,
		jellidation.Field(&r.App, jellidation.NotNil),
		jellidation.Field(&r.ServiceInstance, jellidation.NotNil),
	)
}

type ServiceBindingList struct {
	AppGUIDs             string
	ServiceInstanceGUIDs string
	Include              string
	LabelSelector        labels.Selector
}

func (l *ServiceBindingList) ToMessage() repositories.ListServiceBindingsMessage {
	return repositories.ListServiceBindingsMessage{
		ServiceInstanceGUIDs: parse.ArrayParam(l.ServiceInstanceGUIDs),
		AppGUIDs:             parse.ArrayParam(l.AppGUIDs),
		LabelSelector:        l.LabelSelector,
	}
}

func (l *ServiceBindingList) SupportedKeys() []string {
	return []string{"app_guids", "service_instance_guids", "include", "type", "per_page", "page", "label_selector"}
}

func (l *ServiceBindingList) DecodeFromURLValues(values url.Values) error {
	l.AppGUIDs = values.Get("app_guids")
	l.ServiceInstanceGUIDs = values.Get("service_instance_guids")
	l.Include = values.Get("include")

	labelSelectorRequirements, err := labels.ParseToRequirements(values.Get("label_selector"))
	if err != nil {
		return err
	}

	l.LabelSelector = labels.NewSelector().Add(labelSelectorRequirements...)

	return nil
}

type ServiceBindingUpdate struct {
	Metadata MetadataPatch `json:"metadata"`
}

func (u ServiceBindingUpdate) Validate() error {
	return jellidation.ValidateStruct(&u,
		jellidation.Field(&u.Metadata),
	)
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
