package payloads

import (
	"code.cloudfoundry.org/cf-k8s-api/repositories"
)

// TODO: Make these configurable
var (
// defaultLifecycleType  = "buildpack"
// defaultLifecycleStack = "cflinuxfs3"
)

type RouteCreate struct {
	Host          string             `json:"host" validate:"required"` // TODO: Remove required flag when we support private domains
	Path          string             `json:"path"`
	Relationships RouteRelationships `json:"relationships" validate:"required"`
	Metadata      Metadata           `json:"metadata"`
}

type RouteRelationships struct {
	Domain Relationship `json:"domain" validate:"required"`
	Space  Relationship `json:"space" validate:"required"`
}

func (p RouteCreate) ToRecord() repositories.RouteRecord {
	return repositories.RouteRecord{
		GUID:      "",
		Host:      p.Host,
		Path:      p.Path,
		SpaceGUID: p.Relationships.Space.Data.GUID,
		DomainRef: repositories.DomainRecord{
			GUID: p.Relationships.Domain.Data.GUID,
		},
		Labels:      p.Metadata.Labels,
		Annotations: p.Metadata.Annotations,
		CreatedAt:   "",
		UpdatedAt:   "",
	}
}
