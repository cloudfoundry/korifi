package presenter

import (
	"net/url"
	"time"

	"code.cloudfoundry.org/korifi/api/repositories"
	"code.cloudfoundry.org/korifi/api/repositories/include"
	"code.cloudfoundry.org/korifi/tools"
)

const (
	buildsBase   = "/v3/builds"
	dropletsBase = "/v3/droplets"
)

type BuildResponse struct {
	GUID            string                       `json:"guid"`
	CreatedAt       time.Time                    `json:"created_at"`
	UpdatedAt       time.Time                    `json:"updated_at"`
	CreatedBy       map[string]interface{}       `json:"created_by"`
	State           string                       `json:"state"`
	StagingMemoryMB int                          `json:"staging_memory_in_mb"`
	StagingDiskMB   int                          `json:"staging_disk_in_mb"`
	Error           *string                      `json:"error"`
	Lifecycle       Lifecycle                    `json:"lifecycle"`
	Package         RelationshipData             `json:"package"`
	Droplet         *RelationshipData            `json:"droplet"`
	Relationships   map[string]ToOneRelationship `json:"relationships"`
	Metadata        Metadata                     `json:"metadata"`
	Links           map[string]Link              `json:"links"`
}

func ForBuild(buildRecord repositories.BuildRecord, baseURL url.URL, includes ...include.Resource) BuildResponse {
	toReturn := BuildResponse{
		GUID:            buildRecord.GUID,
		CreatedAt:       tools.ZeroIfNil(toUTC(&buildRecord.CreatedAt)),
		UpdatedAt:       tools.ZeroIfNil(toUTC(buildRecord.UpdatedAt)),
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
		Droplet:       nil,
		Relationships: ForRelationships(buildRecord.Relationships()),
		Metadata: Metadata{
			Labels:      emptyMapIfNil(buildRecord.Labels),
			Annotations: emptyMapIfNil(buildRecord.Annotations),
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
