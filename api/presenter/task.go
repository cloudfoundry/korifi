package presenter

import (
	"net/url"

	"code.cloudfoundry.org/korifi/api/repositories"
)

const (
	tasksBase = "/v3/tasks"
)

type TaskResponse struct {
	Name          string        `json:"name"`
	GUID          string        `json:"guid"`
	Command       string        `json:"command"`
	State         string        `json:"state"`
	Result        TaskResult    `json:"result"`
	Relationships Relationships `json:"relationships"`
	Links         TaskLinks     `json:"links"`
}

type TaskLinks struct {
	Self Link `json:"self"`
	App  Link `json:"app"`
}

type TaskResult struct {
	FailureReason *string `json:"failure_reason"`
}

func ForTask(responseTask repositories.TaskRecord, baseURL url.URL) TaskResponse {
	return TaskResponse{
		Name:    responseTask.Name,
		GUID:    responseTask.GUID,
		Command: responseTask.Command,
		State:   "RUNNING",
		Relationships: Relationships{
			"app": Relationship{
				Data: &RelationshipData{
					GUID: responseTask.AppGUID,
				},
			},
		},
		Links: TaskLinks{
			Self: Link{
				HREF: buildURL(baseURL).appendPath(tasksBase, responseTask.GUID).build(),
			},
			App: Link{
				HREF: buildURL(baseURL).appendPath(appsBase, responseTask.AppGUID).build(),
			},
		},
	}
}
