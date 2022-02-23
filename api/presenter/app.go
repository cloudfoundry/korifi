package presenter

import (
	"errors"
	"net/url"

	"code.cloudfoundry.org/cf-k8s-controllers/api/apierr"
	"code.cloudfoundry.org/cf-k8s-controllers/api/repositories"
)

const (
	appsBase = "/v3/apps"
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

func ForApp(responseApp repositories.AppRecord, baseURL url.URL) AppResponse {
	return AppResponse{
		Name:      responseApp.Name,
		GUID:      responseApp.GUID,
		State:     string(responseApp.State),
		CreatedAt: responseApp.CreatedAt,
		UpdatedAt: responseApp.UpdatedAt,
		Relationships: Relationships{
			"space": Relationship{
				Data: &RelationshipData{
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
				HREF: buildURL(baseURL).appendPath(appsBase, responseApp.GUID).build(),
			},
			Space: Link{
				HREF: buildURL(baseURL).appendPath(spacesBase, responseApp.SpaceGUID).build(),
			},
			Processes: Link{
				HREF: buildURL(baseURL).appendPath(appsBase, responseApp.GUID, "processes").build(),
			},
			Packages: Link{
				HREF: buildURL(baseURL).appendPath(appsBase, responseApp.GUID, "packages").build(),
			},
			EnvironmentVariables: Link{
				HREF: buildURL(baseURL).appendPath(appsBase, responseApp.GUID, "environment_variables").build(),
			},
			CurrentDroplet: Link{
				HREF: buildURL(baseURL).appendPath(appsBase, responseApp.GUID, "droplets/current").build(),
			},
			Droplets: Link{
				HREF: buildURL(baseURL).appendPath(appsBase, responseApp.GUID, "droplets").build(),
			},
			Tasks: Link{
				HREF: buildURL(baseURL).appendPath(appsBase, responseApp.GUID, "tasks").build(),
			},
			StartAction: Link{
				HREF:   buildURL(baseURL).appendPath(appsBase, responseApp.GUID, "actions/start").build(),
				Method: "POST",
			},
			StopAction: Link{
				HREF:   buildURL(baseURL).appendPath(appsBase, responseApp.GUID, "actions/stop").build(),
				Method: "POST",
			},
			Revisions: Link{
				HREF: buildURL(baseURL).appendPath(appsBase, responseApp.GUID, "revisions").build(),
			},
			DeployedRevisions: Link{
				HREF: buildURL(baseURL).appendPath(appsBase, responseApp.GUID, "revisions/deployed").build(),
			},
			Features: Link{
				HREF: buildURL(baseURL).appendPath(appsBase, responseApp.GUID, "features").build(),
			},
		},
	}
}

func ForAppList(appRecordList []repositories.AppRecord, baseURL, requestURL url.URL) ListResponse {
	appResponses := make([]interface{}, 0, len(appRecordList))
	for _, app := range appRecordList {
		appResponses = append(appResponses, ForApp(app, baseURL))
	}

	return ForList(appResponses, baseURL, requestURL)
}

type CurrentDropletResponse struct {
	Relationship `json:",inline"`
	Links        CurrentDropletLinks `json:"links"`
}

type CurrentDropletLinks struct {
	Self    Link `json:"self"`
	Related Link `json:"related"`
}

func ForCurrentDroplet(record repositories.CurrentDropletRecord, baseURL url.URL) CurrentDropletResponse {
	return CurrentDropletResponse{
		Relationship: Relationship{
			Data: &RelationshipData{
				GUID: record.DropletGUID,
			},
		},
		Links: CurrentDropletLinks{
			Self: Link{
				HREF: buildURL(baseURL).appendPath(appsBase, record.AppGUID, "relationships/current_droplet").build(),
			},
			Related: Link{
				HREF: buildURL(baseURL).appendPath(appsBase, record.AppGUID, "droplets/current").build(),
			},
		},
	}
}

type AppEnvVarsResponse struct {
	Var   map[string]string `json:"var"`
	Links AppEnvVarsLinks   `json:"links"`
}

type AppEnvVarsLinks struct {
	Self Link `json:"self"`
	App  Link `json:"app"`
}

func ForAppEnvVars(record repositories.AppEnvVarsRecord, baseURL url.URL) AppEnvVarsResponse {
	return AppEnvVarsResponse{
		Var: record.EnvironmentVariables,
		Links: AppEnvVarsLinks{
			Self: Link{
				HREF: buildURL(baseURL).appendPath(appsBase, record.AppGUID, "environment_variables").build(),
			},
			App: Link{
				HREF: buildURL(baseURL).appendPath(appsBase, record.AppGUID).build(),
			},
		},
	}
}

type AppEnvResponse struct {
	EnvironmentVariables map[string]string `json:"environment_variables"`
	StagingEnvJSON       map[string]string `json:"staging_env_json"`
	RunningEnvJSON       map[string]string `json:"running_env_json"`
	SystemEnvJSON        map[string]string `json:"system_env_json"`
	ApplicationEnvJSON   map[string]string `json:"application_env_json"`
}

func ForAppEnv(envVars map[string]string) AppEnvResponse {
	return AppEnvResponse{
		EnvironmentVariables: envVars,
		StagingEnvJSON:       map[string]string{},
		RunningEnvJSON:       map[string]string{},
		SystemEnvJSON:        map[string]string{},
		ApplicationEnvJSON:   map[string]string{},
	}
}

type ErrorResponse struct {
	StatusCode int
	Body       ErrorsResponse
}

func ForReadError(err error) ErrorResponse {
	var forbiddenErr apierr.ForbiddenError
	if errors.As(err, &forbiddenErr) {
		return ForError(apierr.NewNotFoundError(forbiddenErr.Unwrap(), forbiddenErr.ResourceType()))
	}

	return ForError(err)
}

func ForError(err error) ErrorResponse {
	var apiErr apierr.ApiError
	if errors.As(err, &apiErr) {
		return ErrorResponse{
			StatusCode: apiErr.HttpStatus(),
			Body: ErrorsResponse{
				Errors: []PresentedError{
					{
						Detail: apiErr.Detail(),
						Title:  apiErr.Title(),
						Code:   apiErr.Code(),
					},
				},
			},
		}
	}

	return ForError(apierr.NewUnknownError(err))
}
