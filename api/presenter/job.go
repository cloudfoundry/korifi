package presenter

import "net/url"

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
	Self  Link `json:"self"`
	Space Link `json:"space"`
}

func ForJob(jobGUID string, spaceGUID string, baseURL url.URL) JobResponse {
	return JobResponse{
		GUID:      jobGUID,
		Errors:    nil,
		Warnings:  nil,
		Operation: "space.apply_manifest",
		State:     "COMPLETE",
		CreatedAt: "",
		UpdatedAt: "",
		Links: JobLinks{
			Self: Link{
				HREF: buildURL(baseURL).appendPath("/v3/jobs", jobGUID).build(),
			},
			Space: Link{
				HREF: buildURL(baseURL).appendPath("/v3/spaces", spaceGUID).build(),
			},
		},
	}
}
