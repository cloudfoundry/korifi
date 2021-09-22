package message

import (
	"code.cloudfoundry.org/cf-k8s-api/repositories"
)

// TODO: Make these configurable
var (
// defaultLifecycleType  = "buildpack"
// defaultLifecycleStack = "cflinuxfs3"
)

type RouteCreateMessage struct {
	Host          string             `json:"host" validate:"required"` // TODO: Remove required flag when we support private domains
	Path          string             `json:"path"`
	Relationships RouteRelationships `json:"relationships" validate:"required"`
	Metadata      Metadata           `json:"metadata"`
}

type RouteRelationships struct {
	Domain Relationship `json:"domain" validate:"required"`
	Space  Relationship `json:"space" validate:"required"`
}

func RouteCreateMessageToRouteRecord(requestRoute RouteCreateMessage) repositories.RouteRecord {
	return repositories.RouteRecord{
		GUID:      "",
		SpaceGUID: requestRoute.Relationships.Space.Data.GUID,
		DomainRef: repositories.DomainRecord{
			GUID: requestRoute.Relationships.Domain.Data.GUID,
		},
		Host:        requestRoute.Host,
		Path:        requestRoute.Path,
		Labels:      requestRoute.Metadata.Labels,
		Annotations: requestRoute.Metadata.Annotations,
		CreatedAt:   "",
		UpdatedAt:   "",
	}
}
