package payloads

import (
	"net/url"

	"code.cloudfoundry.org/korifi/api/payloads/parse"
	"code.cloudfoundry.org/korifi/api/payloads/validation"
	"code.cloudfoundry.org/korifi/api/repositories"
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	jellidation "github.com/jellydator/validation"
)

type RouteCreate struct {
	Host          string              `json:"host"`
	Path          string              `json:"path"`
	Relationships *RouteRelationships `json:"relationships"`
	Metadata      Metadata            `json:"metadata"`
}

func (p RouteCreate) Validate() error {
	return jellidation.ValidateStruct(&p,
		jellidation.Field(&p.Host, jellidation.Required),
		jellidation.Field(&p.Relationships, jellidation.NotNil),
		jellidation.Field(&p.Metadata),
	)
}

func (p RouteCreate) ToMessage(domainNamespace, domainName string) repositories.CreateRouteMessage {
	return repositories.CreateRouteMessage{
		Host:            p.Host,
		Path:            p.Path,
		SpaceGUID:       p.Relationships.Space.Data.GUID,
		DomainGUID:      p.Relationships.Domain.Data.GUID,
		DomainNamespace: domainNamespace,
		DomainName:      domainName,
		Labels:          p.Metadata.Labels,
		Annotations:     p.Metadata.Annotations,
	}
}

type RouteRelationships struct {
	Domain Relationship `json:"domain"`
	Space  Relationship `json:"space"`
}

func (r RouteRelationships) Validate() error {
	return jellidation.ValidateStruct(&r,
		jellidation.Field(&r.Domain, validation.StrictlyRequired),
		jellidation.Field(&r.Space, validation.StrictlyRequired),
	)
}

type RouteList struct {
	AppGUIDs    string
	SpaceGUIDs  string
	DomainGUIDs string
	Hosts       string
	Paths       string
}

func (p RouteList) ToMessage() repositories.ListRoutesMessage {
	return repositories.ListRoutesMessage{
		AppGUIDs:    parse.ArrayParam(p.AppGUIDs),
		SpaceGUIDs:  parse.ArrayParam(p.SpaceGUIDs),
		DomainGUIDs: parse.ArrayParam(p.DomainGUIDs),
		Hosts:       parse.ArrayParam(p.Hosts),
		Paths:       parse.ArrayParam(p.Paths),
	}
}

func (p RouteList) SupportedKeys() []string {
	return []string{"app_guids", "space_guids", "domain_guids", "hosts", "paths", "per_page", "page"}
}

func (p *RouteList) DecodeFromURLValues(values url.Values) error {
	p.AppGUIDs = values.Get("app_guids")
	p.SpaceGUIDs = values.Get("space_guids")
	p.DomainGUIDs = values.Get("domain_guids")
	p.Hosts = values.Get("hosts")
	p.Paths = values.Get("paths")
	return nil
}

type RoutePatch struct {
	Metadata MetadataPatch `json:"metadata"`
}

func (p RoutePatch) Validate() error {
	return jellidation.ValidateStruct(&p,
		jellidation.Field(&p.Metadata),
	)
}

func (p RoutePatch) ToMessage(routeGUID, spaceGUID string) repositories.PatchRouteMetadataMessage {
	return repositories.PatchRouteMetadataMessage{
		RouteGUID: routeGUID,
		SpaceGUID: spaceGUID,
		MetadataPatch: repositories.MetadataPatch{
			Annotations: p.Metadata.Annotations,
			Labels:      p.Metadata.Labels,
		},
	}
}

type RouteDestinationCreate struct {
	Destinations []RouteDestination `json:"destinations"`
}

func (r RouteDestinationCreate) Validate() error {
	return jellidation.ValidateStruct(&r,
		jellidation.Field(&r.Destinations),
	)
}

type RouteDestination struct {
	App      AppResource `json:"app"`
	Port     *int        `json:"port"`
	Protocol *string     `json:"protocol"`
}

func (r RouteDestination) Validate() error {
	return jellidation.ValidateStruct(&r,
		jellidation.Field(&r.App),
		jellidation.Field(&r.Protocol, validation.OneOf("http1")),
	)
}

type AppResource struct {
	GUID    string                 `json:"guid"`
	Process *DestinationAppProcess `json:"process"`
}

func (a AppResource) Validate() error {
	return jellidation.ValidateStruct(&a,
		jellidation.Field(&a.GUID, jellidation.Required),
		jellidation.Field(&a.Process),
	)
}

type DestinationAppProcess struct {
	Type string `json:"type"`
}

func (p DestinationAppProcess) Validate() error {
	return jellidation.ValidateStruct(&p,
		jellidation.Field(&p.Type, jellidation.Required),
	)
}

func (dc RouteDestinationCreate) ToMessage(routeRecord repositories.RouteRecord) repositories.AddDestinationsToRouteMessage {
	addDestinations := make([]repositories.DestinationMessage, 0, len(dc.Destinations))
	for _, destination := range dc.Destinations {
		processType := korifiv1alpha1.ProcessTypeWeb
		if destination.App.Process != nil {
			processType = destination.App.Process.Type
		}

		port := 8080
		if destination.Port != nil {
			port = *destination.Port
		}

		protocol := "http1"
		if destination.Protocol != nil {
			protocol = *destination.Protocol
		}

		addDestinations = append(addDestinations, repositories.DestinationMessage{
			AppGUID:     destination.App.GUID,
			ProcessType: processType,
			Port:        port,
			Protocol:    protocol,
		})
	}
	return repositories.AddDestinationsToRouteMessage{
		RouteGUID:            routeRecord.GUID,
		SpaceGUID:            routeRecord.SpaceGUID,
		ExistingDestinations: routeRecord.Destinations,
		NewDestinations:      addDestinations,
	}
}
