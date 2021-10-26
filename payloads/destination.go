package payloads

import (
	"github.com/google/uuid"

	"code.cloudfoundry.org/cf-k8s-api/repositories"
)

type DestinationListCreate struct {
	Destinations []Destination `json:"destinations" validate:"required,dive"`
}

type Destination struct {
	App      *AppResource `json:"app" validate:"required"`
	Port     *int         `json:"port"`
	Protocol string       `json:"protocol" validate:"oneof=http http1"`
}

type AppResource struct {
	GUID    string   `json:"guid" validate:"required"`
	Process *Process `json:"process"`
}

type Process struct {
	Type string `json:"type" validate:"required"`
}

func (dc DestinationListCreate) ToMessage(routeRecord repositories.RouteRecord) repositories.RouteAddDestinationsMessage {
	destinationRecords := make([]repositories.DestinationRecord, 0, len(dc.Destinations))
	for _, destination := range dc.Destinations {
		processType := "web"
		if destination.App.Process != nil {
			processType = destination.App.Process.Type
		}

		port := 8080
		if destination.Port != nil {
			port = *destination.Port
		}

		destinationRecords = append(destinationRecords, repositories.DestinationRecord{
			GUID:        uuid.New().String(),
			AppGUID:     destination.App.GUID,
			ProcessType: processType,
			Port:        port,
		})
	}
	return repositories.RouteAddDestinationsMessage{
		Route:        routeRecord,
		Destinations: destinationRecords,
	}
}
