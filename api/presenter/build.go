package presenter

import (
	"net/url"

	"code.cloudfoundry.org/korifi/api/repositories"
)

const (
	buildsBase   = "/v3/builds"
	dropletsBase = "/v3/droplets"
)

type BuildResponse struct {
	GUID            string                 `json:"guid"`
	CreatedAt       string                 `json:"created_at"`
	UpdatedAt       string                 `json:"updated_at"`
	CreatedBy       map[string]interface{} `json:"created_by"`
	State           string                 `json:"state"`
	StagingMemoryMB int                    `json:"staging_memory_in_mb"`
	StagingDiskMB   int                    `json:"staging_disk_in_mb"`
	Error           *string                `json:"error"`
	Lifecycle       Lifecycle              `json:"lifecycle"`
	Package         RelationshipData       `json:"package"`
	Droplet         *RelationshipData      `json:"droplet"`
	Relationships   Relationships          `json:"relationships"`
	Metadata        Metadata               `json:"metadata"`
	Links           map[string]Link        `json:"links"`
}

func ForBuild(buildRecord repositories.BuildRecord, baseURL url.URL) BuildResponse {
	toReturn := BuildResponse{
		GUID:            buildRecord.GUID,
		CreatedAt:       buildRecord.CreatedAt,
		UpdatedAt:       buildRecord.UpdatedAt,
		CreatedBy:       make(map[string]interface{}),
		State:           buildRecord.State,
		StagingMemoryMB: buildRecord.StagingMemoryMB,
		StagingDiskMB:   buildRecord.StagingDiskMB,
		Lifecycle: Lifecycle{
			Type: buildRecord.Lifecycle.Type,
			Data: LifecycleData{
				Buildpacks: buildRecord.Lifecycle.Data.Buildpacks,
				Stack:      buildRecord.Lifecycle.Data.Stack,
			},
		},
		Package: RelationshipData{
			GUID: buildRecord.PackageGUID,
		},
		Droplet: nil,
		Relationships: Relationships{
			"app": Relationship{
				Data: &RelationshipData{
					GUID: buildRecord.AppGUID,
				},
			},
		},
		Metadata: Metadata{
			Labels:      map[string]string{},
			Annotations: map[string]string{},
		},
		Links: map[string]Link{
			"self": {
				HRef: buildURL(baseURL).appendPath(buildsBase, buildRecord.GUID).build(),
			},
			"app": {
				HRef: buildURL(baseURL).appendPath(appsBase, buildRecord.AppGUID).build(),
			},
		},
	}

	if buildRecord.DropletGUID != "" {
		toReturn.Droplet = &RelationshipData{
			GUID: buildRecord.DropletGUID,
		}

		toReturn.Links["droplet"] = Link{
			HRef: buildURL(baseURL).appendPath(dropletsBase, buildRecord.DropletGUID).build(),
		}
	}

	if buildRecord.StagingErrorMsg != "" {
		toReturn.Error = &buildRecord.StagingErrorMsg
	}

	return toReturn
}

func ForBuildList(buildRecordList []repositories.BuildRecord, baseURL, requestURL url.URL) ListResponse {
	buildResponses := make([]interface{}, 0, len(buildRecordList))
	for _, build := range buildRecordList {
		buildResponses = append(buildResponses, ForBuild(build, baseURL))
	}

	return ForList(buildResponses, baseURL, requestURL)
}
