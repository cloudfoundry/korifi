package presenter

import (
	"code.cloudfoundry.org/cf-k8s-api/repositories"
	"fmt"
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

func ForBuild(buildRecord repositories.BuildRecord, baseURL string) BuildResponse {
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
				Data: RelationshipData{
					GUID: buildRecord.AppGUID,
				},
			},
		},
		Metadata: Metadata{
			Labels:      map[string]string{},
			Annotations: map[string]string{},
		},
		Links: map[string]Link{
			"self": Link{
				HREF: prefixedLinkURL(baseURL, fmt.Sprintf("v3/builds/%s", buildRecord.GUID)),
			},
			"app": Link{
				HREF: prefixedLinkURL(baseURL, fmt.Sprintf("v3/apps/%s", buildRecord.AppGUID)),
			},
		},
	}

	if buildRecord.DropletGUID != "" {
		toReturn.Droplet = &RelationshipData{
			GUID: buildRecord.DropletGUID,
		}

		toReturn.Links["droplet"] = Link{
			HREF: prefixedLinkURL(baseURL, fmt.Sprintf("v3/droplets/%s", buildRecord.DropletGUID)),
		}
	}

	if buildRecord.StagingErrorMsg != "" {
		toReturn.Error = &buildRecord.StagingErrorMsg
	}

	return toReturn
}
