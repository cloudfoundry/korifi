package presenter

import (
	"net/url"

	"code.cloudfoundry.org/korifi/api/repositories"
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
			Labels:      emptyMapIfNil(responseApp.Labels),
			Annotations: emptyMapIfNil(responseApp.Annotations),
		},
		Links: AppLinks{
			Self: Link{
				HRef: buildURL(baseURL).appendPath(appsBase, responseApp.GUID).build(),
			},
			Space: Link{
				HRef: buildURL(baseURL).appendPath(spacesBase, responseApp.SpaceGUID).build(),
			},
			Processes: Link{
				HRef: buildURL(baseURL).appendPath(appsBase, responseApp.GUID, "processes").build(),
			},
			Packages: Link{
				HRef: buildURL(baseURL).appendPath(appsBase, responseApp.GUID, "packages").build(),
			},
			EnvironmentVariables: Link{
				HRef: buildURL(baseURL).appendPath(appsBase, responseApp.GUID, "environment_variables").build(),
			},
			CurrentDroplet: Link{
				HRef: buildURL(baseURL).appendPath(appsBase, responseApp.GUID, "droplets/current").build(),
			},
			Droplets: Link{
				HRef: buildURL(baseURL).appendPath(appsBase, responseApp.GUID, "droplets").build(),
			},
			Tasks: Link{
				HRef: buildURL(baseURL).appendPath(appsBase, responseApp.GUID, "tasks").build(),
			},
			StartAction: Link{
				HRef:   buildURL(baseURL).appendPath(appsBase, responseApp.GUID, "actions/start").build(),
				Method: "POST",
			},
			StopAction: Link{
				HRef:   buildURL(baseURL).appendPath(appsBase, responseApp.GUID, "actions/stop").build(),
				Method: "POST",
			},
			Revisions: Link{
				HRef: buildURL(baseURL).appendPath(appsBase, responseApp.GUID, "revisions").build(),
			},
			DeployedRevisions: Link{
				HRef: buildURL(baseURL).appendPath(appsBase, responseApp.GUID, "revisions/deployed").build(),
			},
			Features: Link{
				HRef: buildURL(baseURL).appendPath(appsBase, responseApp.GUID, "features").build(),
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
				HRef: buildURL(baseURL).appendPath(appsBase, record.AppGUID, "relationships/current_droplet").build(),
			},
			Related: Link{
				HRef: buildURL(baseURL).appendPath(appsBase, record.AppGUID, "droplets/current").build(),
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
				HRef: buildURL(baseURL).appendPath(appsBase, record.AppGUID, "environment_variables").build(),
			},
			App: Link{
				HRef: buildURL(baseURL).appendPath(appsBase, record.AppGUID).build(),
			},
		},
	}
}

type AppEnvResponse struct {
	EnvironmentVariables map[string]string      `json:"environment_variables"`
	StagingEnvJSON       map[string]string      `json:"staging_env_json"`
	RunningEnvJSON       map[string]string      `json:"running_env_json"`
	SystemEnvJSON        map[string]interface{} `json:"system_env_json"`
	ApplicationEnvJSON   map[string]string      `json:"application_env_json"`
}

func ForAppEnv(envVarRecord repositories.AppEnvRecord) AppEnvResponse {
	return AppEnvResponse{
		EnvironmentVariables: envVarRecord.EnvironmentVariables,
		StagingEnvJSON:       map[string]string{},
		RunningEnvJSON:       map[string]string{},
		SystemEnvJSON:        envVarRecord.SystemEnv,
		ApplicationEnvJSON:   map[string]string{},
	}
}
