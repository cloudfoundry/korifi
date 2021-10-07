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

type AppListResponse struct {
	PaginationData PaginationData `json:"pagination"`
	Resources      []AppResponse  `json:"resources"`
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
				Data: RelationshipData{
					GUID: responseApp.SpaceGUID,
				},
			},
		},
		Lifecycle: Lifecycle{
			Type: responseApp.Lifecycle.Type,
			Data: LifecycleData{
				Buildpacks: responseApp.Lifecycle.Data.Buildpacks,
				Stack:      responseApp.Lifecycle.Data.Stack,
			},
		},
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
			Tasks: Link{
				HREF: prefixedLinkURL(baseURL, fmt.Sprintf("v3/apps/%s/tasks", responseApp.GUID)),
			},
			StartAction: Link{
				HREF:   prefixedLinkURL(baseURL, fmt.Sprintf("v3/apps/%s/actions/start", responseApp.GUID)),
				Method: "POST",
			},
			StopAction: Link{
				HREF:   prefixedLinkURL(baseURL, fmt.Sprintf("v3/apps/%s/actions/stop", responseApp.GUID)),
				Method: "POST",
			},
			Revisions: Link{
				HREF: prefixedLinkURL(baseURL, fmt.Sprintf("v3/apps/%s/revisions", responseApp.GUID)),
			},
			DeployedRevisions: Link{
				HREF: prefixedLinkURL(baseURL, fmt.Sprintf("v3/apps/%s/revisions/deployed", responseApp.GUID)),
			},
			Features: Link{
				HREF: prefixedLinkURL(baseURL, fmt.Sprintf("v3/apps/%s/features", responseApp.GUID)),
			},
		},
	}
}

func ForAppList(appRecordList []repositories.AppRecord, baseURL string) AppListResponse {
	appResponses := make([]AppResponse, 0, len(appRecordList))
	for _, app := range appRecordList {
		appResponses = append(appResponses, ForApp(app, baseURL))
	}

	appListResponse := AppListResponse{
		PaginationData: PaginationData{
			TotalResults: len(appResponses),
			TotalPages:   1,
			First: PageRef{
				HREF: prefixedLinkURL(baseURL, "v3/apps?page=1"),
			},
			Last: PageRef{
				HREF: prefixedLinkURL(baseURL, "v3/apps?page=1"),
			},
		},
		Resources: appResponses,
	}

	return appListResponse
}

type CurrentDropletResponse struct {
	Relationship `json:",inline"`
	Links        CurrentDropletLinks `json:"links"`
}

type CurrentDropletLinks struct {
	Self    Link `json:"self"`
	Related Link `json:"related"`
}

func ForCurrentDroplet(record repositories.CurrentDropletRecord, baseURL string) CurrentDropletResponse {
	return CurrentDropletResponse{
		Relationship: Relationship{
			Data: RelationshipData{
				GUID: record.DropletGUID,
			},
		},
		Links: CurrentDropletLinks{
			Self: Link{
				HREF: prefixedLinkURL(baseURL, fmt.Sprintf("v3/apps/%s/relationships/current_droplet", record.AppGUID)),
			},
			Related: Link{
				HREF: prefixedLinkURL(baseURL, fmt.Sprintf("v3/apps/%s/droplets/current", record.AppGUID)),
			},
		},
	}
}
