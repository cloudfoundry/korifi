package presenter

import (
	"fmt"
	"net/url"
)

const (
	JobGUIDDelimiter = "~"

	StateComplete   = "COMPLETE"
	StateFailed     = "FAILED"
	StateProcessing = "PROCESSING"

	AppDeleteOperation          = "app.delete"
	OrgDeleteOperation          = "org.delete"
	RouteDeleteOperation        = "route.delete"
	SpaceApplyManifestOperation = "space.apply_manifest"
	SpaceDeleteOperation        = "space.delete"
	DomainDeleteOperation       = "domain.delete"
	RoleDeleteOperation         = "role.delete"
)

type JobResponseError struct {
	Detail string `json:"detail"`
	Title  string `json:"title"`
	Code   int    `json:"code"`
}

type JobResponse struct {
	GUID      string             `json:"guid"`
	Errors    []JobResponseError `json:"errors"`
	Warnings  *string            `json:"warnings"`
	Operation string             `json:"operation"`
	State     string             `json:"state"`
	CreatedAt string             `json:"created_at"`
	UpdatedAt string             `json:"updated_at"`
	Links     JobLinks           `json:"links"`
}

type JobLinks struct {
	Self  Link  `json:"self"`
	Space *Link `json:"space,omitempty"`
}

func ForManifestApplyJob(jobGUID string, spaceGUID string, baseURL url.URL) JobResponse {
	response := ForJob(jobGUID, []JobResponseError{}, StateComplete, SpaceApplyManifestOperation, baseURL)
	response.Links.Space = &Link{
		HRef: buildURL(baseURL).appendPath("/v3/spaces", spaceGUID).build(),
	}
	return response
}

func ForJob(jobGUID string, errors []JobResponseError, state string, operation string, baseURL url.URL) JobResponse {
	return JobResponse{
		GUID:      jobGUID,
		Errors:    errors,
		Warnings:  nil,
		Operation: operation,
		State:     state,
		CreatedAt: "",
		UpdatedAt: "",
		Links: JobLinks{
			Self: Link{
				HRef: buildURL(baseURL).appendPath("/v3/jobs", jobGUID).build(),
			},
		},
	}
}

func JobURLForRedirects(resourceGUID string, operation string, baseURL url.URL) string {
	jobGUID := fmt.Sprintf("%s%s%s", operation, JobGUIDDelimiter, resourceGUID)
	return buildURL(baseURL).appendPath("/v3/jobs", jobGUID).build()
}
