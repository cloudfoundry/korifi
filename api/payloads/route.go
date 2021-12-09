package payloads

import (
	"code.cloudfoundry.org/cf-k8s-controllers/api/repositories"
)

type RouteCreate struct {
	Host          string             `json:"host" validate:"hostname_rfc1123,required"` // TODO: Not required when we support private domains
	Path          string             `json:"path" validate:"routepathstartswithslash"`
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
		Domain: repositories.DomainRecord{
			GUID: p.Relationships.Domain.Data.GUID,
		},
		Labels:      p.Metadata.Labels,
		Annotations: p.Metadata.Annotations,
		CreatedAt:   "",
		UpdatedAt:   "",
	}
}

type RouteList struct {
	AppGUIDs    string `schema:"app_guids"`
	SpaceGUIDs  string `schema:"space_guids"`
	DomainGUIDs string `schema:"domain_guids"`
	Hosts       string `schema:"hosts"`
	Paths       string `schema:"paths"`
}

func (p *RouteList) ToMessage() repositories.FetchRouteListMessage {
	return repositories.FetchRouteListMessage{
		AppGUIDs:    parseArrayParam(p.AppGUIDs),
		SpaceGUIDs:  parseArrayParam(p.SpaceGUIDs),
		DomainGUIDs: parseArrayParam(p.DomainGUIDs),
		Hosts:       parseArrayParam(p.Hosts),
		Paths:       parseArrayParam(p.Paths),
	}
}

func (p *RouteList) SupportedFilterKeys() []string {
	return []string{"app_guids", "space_guids", "domain_guids", "hosts", "paths"}
}
