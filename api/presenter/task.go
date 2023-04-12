package presenter

import (
	"net/http"
	"net/url"

	"code.cloudfoundry.org/korifi/api/repositories"
)

const (
	tasksBase = "/v3/tasks"
)

type TaskResponse struct {
	Name          string        `json:"name"`
	GUID          string        `json:"guid"`
	Command       string        `json:"command,omitempty"`
	DropletGUID   string        `json:"droplet_guid"`
	Metadata      Metadata      `json:"metadata"`
	Relationships Relationships `json:"relationships"`
	Links         TaskLinks     `json:"links"`
	SequenceID    int64         `json:"sequence_id"`
	CreatedAt     string        `json:"created_at"`
	UpdatedAt     string        `json:"updated_at"`
	MemoryMB      int64         `json:"memory_in_mb"`
	DiskMB        int64         `json:"disk_in_mb"`
	State         string        `json:"state"`
	Result        TaskResult    `json:"result"`
}

type TaskResult struct {
	FailureReason *string `json:"failure_reason"`
}

type TaskLinks struct {
	Self    Link `json:"self"`
	App     Link `json:"app"`
	Droplet Link `json:"droplet"`
	Cancel  Link `json:"cancel"`
}

func ForTask(responseTask repositories.TaskRecord, baseURL url.URL) TaskResponse {
	result := TaskResult{}

	if responseTask.FailureReason != "" {
		result.FailureReason = &responseTask.FailureReason
	}

	return TaskResponse{
		Name:        responseTask.Name,
		GUID:        responseTask.GUID,
		Command:     responseTask.Command,
		SequenceID:  responseTask.SequenceID,
		DropletGUID: responseTask.DropletGUID,
		CreatedAt:   responseTask.CreatedAt,
		UpdatedAt:   responseTask.UpdatedAt,
		MemoryMB:    responseTask.MemoryMB,
		DiskMB:      responseTask.DiskMB,
		State:       responseTask.State,
		Result:      result,
		Metadata: Metadata{
			Labels:      emptyMapIfNil(responseTask.Labels),
			Annotations: emptyMapIfNil(responseTask.Annotations),
		},
		Relationships: Relationships{
			"app": Relationship{
				Data: &RelationshipData{
					GUID: responseTask.AppGUID,
				},
			},
		},
		Links: TaskLinks{
			Self: Link{
				HRef: buildURL(baseURL).appendPath(tasksBase, responseTask.GUID).build(),
			},
			App: Link{
				HRef: buildURL(baseURL).appendPath(appsBase, responseTask.AppGUID).build(),
			},
			Droplet: Link{
				HRef: buildURL(baseURL).appendPath(dropletsBase, responseTask.DropletGUID).build(),
			},
			Cancel: Link{
				HRef:   buildURL(baseURL).appendPath(tasksBase, responseTask.GUID, "actions", "cancel").build(),
				Method: http.MethodPost,
			},
		},
	}
}
