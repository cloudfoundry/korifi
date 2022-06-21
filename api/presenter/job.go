package presenter

import (
	"fmt"
	"net/url"
)

const (
	JobGUIDDelimiter = "~"

	AppDeleteOperation          = "app.delete"
	OrgDeleteOperation          = "org.delete"
	RouteDeleteOperation        = "route.delete"
	SpaceApplyManifestOperation = "space.apply_manifest"
	SpaceDeleteOperation        = "space.delete"
)

type JobResponse struct {
	GUID      string   `json:"guid"`
	Errors    *string  `json:"errors"`
	Warnings  *string  `json:"warnings"`
	Operation string   `json:"operation"`
	State     string   `json:"state"`
	CreatedAt string   `json:"created_at"`
	UpdatedAt string   `json:"updated_at"`
	Links     JobLinks `json:"links"`
}

type JobLinks struct {
	Self  Link  `json:"self"`
	Space *Link `json:"space,omitempty"`
}

func ForManifestApplyJob(jobGUID string, spaceGUID string, baseURL url.URL) JobResponse {
	return JobResponse{
		GUID:      jobGUID,
		Errors:    nil,
		Warnings:  nil,
		Operation: SpaceApplyManifestOperation,
		State:     "COMPLETE",
		CreatedAt: "",
		UpdatedAt: "",
		Links: JobLinks{
			Self: Link{
				HRef: buildURL(baseURL).appendPath("/v3/jobs", jobGUID).build(),
			},
			Space: &Link{
				HRef: buildURL(baseURL).appendPath("/v3/spaces", spaceGUID).build(),
			},
		},
	}
}

func ForDeleteJob(jobGUID string, operation string, baseURL url.URL) JobResponse {
	return JobResponse{
		GUID:      jobGUID,
		Errors:    nil,
		Warnings:  nil,
		Operation: operation,
		State:     "COMPLETE",
		CreatedAt: "",
		UpdatedAt: "",
		Links: JobLinks{
			Self: Link{
				HRef: buildURL(baseURL).appendPath("/v3/jobs", jobGUID).build(),
			},
		},
	}
}

func JobURLForRedirects(resourceName string, operation string, baseURL url.URL) string {
	jobGUID := fmt.Sprintf("%s%s%s", operation, JobGUIDDelimiter, resourceName)
	return buildURL(baseURL).appendPath("/v3/jobs", jobGUID).build()
}
