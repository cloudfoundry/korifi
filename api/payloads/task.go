package payloads

import (
	"code.cloudfoundry.org/korifi/api/repositories"
)

type TaskCreate struct {
	Command string `json:"command" validate:"required"`
}

func (p TaskCreate) ToMessage(appRecord repositories.AppRecord) repositories.CreateTaskMessage {
	return repositories.CreateTaskMessage{
		Command:   p.Command,
		SpaceGUID: appRecord.SpaceGUID,
		AppGUID:   appRecord.GUID,
	}
}
