package payloads

import (
	"net/url"
	"strconv"
	"strings"

	"code.cloudfoundry.org/korifi/api/payloads/validation"
	"code.cloudfoundry.org/korifi/api/repositories"
	jellidation "github.com/jellydator/validation"
)

type TaskCreate struct {
	Command  string   `json:"command"`
	Metadata Metadata `json:"metadata"`
}

func (c TaskCreate) Validate() error {
	return jellidation.ValidateStruct(&c,
		jellidation.Field(&c.Command, jellidation.Required),
		jellidation.Field(&c.Metadata),
	)
}

func (p TaskCreate) ToMessage(appRecord repositories.AppRecord) repositories.CreateTaskMessage {
	return repositories.CreateTaskMessage{
		Command:   p.Command,
		SpaceGUID: appRecord.SpaceGUID,
		AppGUID:   appRecord.GUID,
		Metadata:  repositories.Metadata(p.Metadata),
	}
}

type TaskList struct {
	OrderBy    string
	Pagination Pagination
}

func (l *TaskList) SupportedKeys() []string {
	return []string{
		"order_by",
		"page",
		"per_page",
	}
}

func (l TaskList) Validate() error {
	return jellidation.ValidateStruct(&l,
		jellidation.Field(&l.OrderBy, validation.OneOfOrderBy("created_at", "updated_at")),
		jellidation.Field(&l.Pagination),
	)
}

func (l *TaskList) DecodeFromURLValues(values url.Values) error {
	l.OrderBy = values.Get("order_by")
	return l.Pagination.DecodeFromURLValues(values)
}

func (l *TaskList) ToMessage() repositories.ListTasksMessage {
	return repositories.ListTasksMessage{
		OrderBy:    l.OrderBy,
		Pagination: l.Pagination.ToMessage(DefaultPageSize),
	}
}

type AppTaskList struct {
	SequenceIDs []int64
	OrderBy     string
	Pagination  Pagination
}

func (t *AppTaskList) ToMessage() repositories.ListTasksMessage {
	return repositories.ListTasksMessage{
		SequenceIDs: t.SequenceIDs,
		OrderBy:     t.OrderBy,
		Pagination:  t.Pagination.ToMessage(DefaultPageSize),
	}
}

func (t *AppTaskList) SupportedKeys() []string {
	return []string{"sequence_ids", "per_page", "page", "order_by"}
}

func (l AppTaskList) Validate() error {
	return jellidation.ValidateStruct(&l,
		jellidation.Field(&l.OrderBy, validation.OneOfOrderBy("created_at", "updated_at")),
		jellidation.Field(&l.Pagination),
	)
}

func (a *AppTaskList) DecodeFromURLValues(values url.Values) error {
	idsStr := values.Get("sequence_ids")

	var ids []int64
	for _, idStr := range strings.Split(idsStr, ",") {
		if idStr == "" {
			continue
		}
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			return err
		}
		ids = append(ids, id)
	}

	a.SequenceIDs = ids
	a.OrderBy = values.Get("order_by")
	return a.Pagination.DecodeFromURLValues(values)
}

type TaskUpdate struct {
	Metadata MetadataPatch `json:"metadata"`
}

func (u TaskUpdate) Validate() error {
	return jellidation.ValidateStruct(&u,
		jellidation.Field(&u.Metadata),
	)
}

func (u *TaskUpdate) ToMessage(taskGUID, spaceGUID string) repositories.PatchTaskMetadataMessage {
	return repositories.PatchTaskMetadataMessage{
		TaskGUID:  taskGUID,
		SpaceGUID: spaceGUID,
		MetadataPatch: repositories.MetadataPatch{
			Annotations: u.Metadata.Annotations,
			Labels:      u.Metadata.Labels,
		},
	}
}
