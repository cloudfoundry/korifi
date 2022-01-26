package payloads

import "code.cloudfoundry.org/cf-k8s-controllers/api/repositories"

type ServiceInstanceCreate struct {
	Name          string                       `json:"name" validate:"required"`
	Type          string                       `json:"type" validate:"required,oneof=user-provided"`
	Tags          []string                     `json:"tags" validate:"serviceinstancetaglength"`
	Credentials   map[string]string            `json:"credentials"`
	Relationships ServiceInstanceRelationships `json:"relationships" validate:"required"`
	Metadata      Metadata                     `json:"metadata"`
}

type ServiceInstanceRelationships struct {
	Space Relationship `json:"space" validate:"required"`
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
