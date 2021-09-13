package presenter

import (
	"fmt"

	"code.cloudfoundry.org/cf-k8s-api/repositories"
)

type AppResponse struct {
	Name  string `json:"name"`
	GUID  string `json:"guid"`
	State string `json:"state"`

	CreatedAt     string        `json:"created_at"`
	UpdatedAt     string        `json:"updated_at"`
	Relationships Relationships `json:"relationships"`
	Lifecycle     Lifecycle     `json:"lifecycle"`
	Metadata      Metadata      `json:"metadata"`
	Links         AppLinks      `json:"links"`
}

type AppLinks struct {
	Self                 Link `json:"self"`
	Space                Link `json:"space"`
	Processes            Link `json:"processes"`
	Packages             Link `json:"packages"`
	EnvironmentVariables Link `json:"environment_variables"`
	CurrentDroplet       Link `json:"current_droplet"`
	Droplets             Link `json:"droplets"`
	Tasks                Link `json:"tasks"`
	StartAction          Link `json:"start"`
	StopAction           Link `json:"stop"`
	Revisions            Link `json:"revisions"`
	DeployedRevisions    Link `json:"deployed_revisions"`
	Features             Link `json:"features"`
}

func ForApp(responseApp repositories.AppRecord, baseURL string) AppResponse {
	return AppResponse{
		Name:      responseApp.Name,
		GUID:      responseApp.GUID,
		State:     string(responseApp.State),
		CreatedAt: responseApp.CreatedAt,
		UpdatedAt: responseApp.UpdatedAt,
		Relationships: Relationships{
			"space": Relationship{
				GUID: responseApp.SpaceGUID,
			},
		},
		Lifecycle: Lifecycle{LifecycleData{
			Buildpacks: responseApp.Lifecycle.Data.Buildpacks,
			Stack:      responseApp.Lifecycle.Data.Stack,
		}},
		Metadata: Metadata{
			Labels:      map[string]string{},
			Annotations: map[string]string{},
		},
		Links: AppLinks{
			Self: Link{
				HREF: prefixedLinkURL(baseURL, fmt.Sprintf("v3/apps/%s", responseApp.GUID)),
			},
			Space: Link{
				HREF: prefixedLinkURL(baseURL, fmt.Sprintf("v3/spaces/%s", responseApp.SpaceGUID)),
			},
			Processes: Link{
				HREF: prefixedLinkURL(baseURL, fmt.Sprintf("v3/apps/%s/processes", responseApp.GUID)),
			},
			Packages: Link{
				HREF: prefixedLinkURL(baseURL, fmt.Sprintf("v3/apps/%s/packages", responseApp.GUID)),
			},
			EnvironmentVariables: Link{
				HREF: prefixedLinkURL(baseURL, fmt.Sprintf("v3/apps/%s/environment_variables", responseApp.GUID)),
			},
			CurrentDroplet: Link{
				HREF: prefixedLinkURL(baseURL, fmt.Sprintf("v3/apps/%s/droplets/current", responseApp.GUID)),
			},
			Droplets: Link{
				HREF: prefixedLinkURL(baseURL, fmt.Sprintf("v3/apps/%s/droplets", responseApp.GUID)),
			},
			Tasks: Link{},
			StartAction: Link{
				HREF:   prefixedLinkURL(baseURL, fmt.Sprintf("v3/apps/%s/actions/start", responseApp.GUID)),
				Method: "POST",
			},
			StopAction: Link{
				HREF:   prefixedLinkURL(baseURL, fmt.Sprintf("v3/apps/%s/actions/stop", responseApp.GUID)),
				Method: "POST",
			},
			Revisions:         Link{},
			DeployedRevisions: Link{},
			Features:          Link{},
		},
	}
}
