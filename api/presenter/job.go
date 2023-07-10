package presenter

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
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

var (
	jobOperationPattern       = `(([a-z_\-]+)\.([a-z_]+))` // (e.g. app.delete, space.apply_manifest, etc.)
	resourceIdentifierPattern = `([A-Za-z0-9\-\.]+)`       // (e.g. cf-space-a4cd478b-0b02-452f-8498-ce87ec5c6649, CUSTOM_ORG_ID, etc.)
	jobRegexp                 = regexp.MustCompile(jobOperationPattern + JobGUIDDelimiter + resourceIdentifierPattern)
)

type Job struct {
	GUID         string
	Type         string
	ResourceGUID string
	ResourceType string
}

func JobFromGUID(guid string) (Job, bool) {
	matches := jobRegexp.FindStringSubmatch(guid)

	if len(matches) != 5 {
		return Job{}, false
	} else {
		return Job{
			GUID:         guid,
			Type:         matches[1],
			ResourceType: strings.Title(matches[2]),
			ResourceGUID: matches[4],
		}, true
	}
}

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

func ForManifestApplyJob(job Job, baseURL url.URL) JobResponse {
	response := ForJob(job, []JobResponseError{}, StateComplete, baseURL)
	response.Links.Space = &Link{
		HRef: buildURL(baseURL).appendPath("/v3/spaces", job.ResourceGUID).build(),
	}
	return response
}

func ForJob(job Job, errors []JobResponseError, state string, baseURL url.URL) JobResponse {
	return JobResponse{
		GUID:      job.GUID,
		Errors:    errors,
		Warnings:  nil,
		Operation: job.Type,
		State:     state,
		CreatedAt: "",
		UpdatedAt: "",
		Links: JobLinks{
			Self: Link{
				HRef: buildURL(baseURL).appendPath("/v3/jobs", job.GUID).build(),
			},
		},
	}
}

func JobURLForRedirects(resourceGUID string, operation string, baseURL url.URL) string {
	jobGUID := fmt.Sprintf("%s%s%s", operation, JobGUIDDelimiter, resourceGUID)
	return buildURL(baseURL).appendPath("/v3/jobs", jobGUID).build()
}
