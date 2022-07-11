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

type TaskList struct {
	SequenceIDs []int64 `schema:"sequence_ids"`
}

func (t *TaskList) ToMessage() repositories.ListTaskMessage {
	return repositories.ListTaskMessage{
		SequenceIDs: t.SequenceIDs,
	}
}

func (t *TaskList) SupportedKeys() []string {
	return []string{"sequence_ids"}
}
