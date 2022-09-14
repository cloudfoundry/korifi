package payloads

import (
	"code.cloudfoundry.org/korifi/api/repositories"
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
)

type DestinationListCreate struct {
	Destinations []Destination `json:"destinations" validate:"required,dive"`
}

type Destination struct {
	App      *AppResource `json:"app" validate:"required"`
	Port     *int         `json:"port"`
	Protocol *string      `json:"protocol" validate:"omitempty,oneof=http1"`
}

type AppResource struct {
	GUID    string                 `json:"guid" validate:"required"`
	Process *DestinationAppProcess `json:"process"`
}

type DestinationAppProcess struct {
	Type string `json:"type" validate:"required"`
}

func (dc DestinationListCreate) ToMessage(routeRecord repositories.RouteRecord) repositories.AddDestinationsToRouteMessage {
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
