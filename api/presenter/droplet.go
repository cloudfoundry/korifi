package presenter

import (
	"net/url"

	"code.cloudfoundry.org/korifi/api/repositories"
	"code.cloudfoundry.org/korifi/api/repositories/include"
	"code.cloudfoundry.org/korifi/tools"
)

type DropletResponse struct {
	GUID              string                       `json:"guid"`
	CreatedAt         string                       `json:"created_at"`
	UpdatedAt         string                       `json:"updated_at"`
	State             string                       `json:"state"`
	Error             *string                      `json:"error"`
	Lifecycle         Lifecycle                    `json:"lifecycle"`
	ExecutionMetadata string                       `json:"execution_metadata"`
	Checksum          *ChecksumData                `json:"checksum"`
	Buildpacks        []BuildpackData              `json:"buildpacks"`
	ProcessTypes      map[string]string            `json:"process_types"`
	Stack             string                       `json:"stack"`
	Image             *string                      `json:"image"`
	Relationships     map[string]ToOneRelationship `json:"relationships"`
	Metadata          Metadata                     `json:"metadata"`
	Links             map[string]*Link             `json:"links"`
}

type ChecksumData struct {
	Type  string `json:"type"`
	Value string `json:"value"`
}

type BuildpackData struct {
	Name          string `json:"name"`
	DetectOutput  string `json:"detect_output"`
	BuildpackName string `json:"buildpack_name"`
	Version       string `json:"version"`
}

func ForDroplet(dropletRecord repositories.DropletRecord, baseURL url.URL, includes ...include.Resource) DropletResponse {
	toReturn := DropletResponse{
		GUID:      dropletRecord.GUID,
		CreatedAt: tools.ZeroIfNil(formatTimestamp(&dropletRecord.CreatedAt)),
		UpdatedAt: tools.ZeroIfNil(formatTimestamp(dropletRecord.UpdatedAt)),
		State:     dropletRecord.State,
		Lifecycle: Lifecycle{
			Type: dropletRecord.Lifecycle.Type,
			Data: LifecycleData{
				Buildpacks: emptySliceIfNil(dropletRecord.Lifecycle.Data.Buildpacks),
				Stack:      dropletRecord.Lifecycle.Data.Stack,
			},
		},
		ExecutionMetadata: "",
		Buildpacks:        []BuildpackData{},
		ProcessTypes:      dropletRecord.ProcessTypes,
		Stack:             dropletRecord.Stack,
		Relationships:     ForRelationships(dropletRecord.Relationships()),
		Metadata: Metadata{
			Labels:      emptyMapIfNil(dropletRecord.Labels),
			Annotations: emptyMapIfNil(dropletRecord.Annotations),
		},
		Links: map[string]*Link{
			"self": {
				HRef: buildURL(baseURL).appendPath(dropletsBase, dropletRecord.GUID).build(),
			},
			"package": {
				HRef: buildURL(baseURL).appendPath(packagesBase, dropletRecord.PackageGUID).build(),
			},
			"app": {
				HRef: buildURL(baseURL).appendPath(appsBase, dropletRecord.AppGUID).build(),
			},
			"assign_current_droplet": {
				HRef:   buildURL(baseURL).appendPath(appsBase, dropletRecord.AppGUID, "relationships/current_droplet").build(),
				Method: "PATCH",
			},
			"download": nil,
		},
	}
	if dropletRecord.DropletErrorMsg != "" {
		toReturn.Error = &dropletRecord.DropletErrorMsg
	}
	if dropletRecord.Lifecycle.Type == "docker" {
		toReturn.Image = &dropletRecord.Image
	}
	return toReturn
}
