package presenter

import (
	"fmt"

	"code.cloudfoundry.org/cf-k8s-api/repositories"
)

type DropletResponse struct {
	GUID              string            `json:"guid"`
	CreatedAt         string            `json:"created_at"`
	UpdatedAt         string            `json:"updated_at"`
	State             string            `json:"state"`
	Error             *string           `json:"error"`
	Lifecycle         Lifecycle         `json:"lifecycle"`
	ExecutionMetadata string            `json:"execution_metadata"`
	Checksum          *ChecksumData     `json:"checksum"`
	Buildpacks        []BuildpackData   `json:"buildpacks"`
	ProcessTypes      map[string]string `json:"process_types"`
	Stack             string            `json:"stack"`
	Image             *string           `json:"image"`
	Relationships     Relationships     `json:"relationships"`
	Metadata          Metadata          `json:"metadata"`
	Links             map[string]*Link  `json:"links"`
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

func ForDroplet(dropletRecord repositories.DropletRecord, baseURL string) DropletResponse {
	toReturn := DropletResponse{
		GUID:      dropletRecord.GUID,
		CreatedAt: dropletRecord.CreatedAt,
		UpdatedAt: dropletRecord.UpdatedAt,
		State:     dropletRecord.State,
		Lifecycle: Lifecycle{
			Type: dropletRecord.Lifecycle.Type,
			Data: LifecycleData{
				Buildpacks: dropletRecord.Lifecycle.Data.Buildpacks,
				Stack:      dropletRecord.Lifecycle.Data.Stack,
			},
		},
		ExecutionMetadata: "",
		Buildpacks:        []BuildpackData{},
		ProcessTypes:      dropletRecord.ProcessTypes,
		Stack:             dropletRecord.Stack,
		Relationships: Relationships{
			"app": Relationship{
				Data: RelationshipData{
					GUID: dropletRecord.AppGUID,
				},
			},
		},
		Metadata: Metadata{
			Labels:      map[string]string{},
			Annotations: map[string]string{},
		},
		Links: map[string]*Link{
			"self": {
				HREF: prefixedLinkURL(baseURL, fmt.Sprintf("v3/droplets/%s", dropletRecord.GUID)),
			},
			"package": {
				HREF: prefixedLinkURL(baseURL, fmt.Sprintf("v3/packages/%s", dropletRecord.PackageGUID)),
			},
			"app": {
				HREF: prefixedLinkURL(baseURL, fmt.Sprintf("v3/apps/%s", dropletRecord.AppGUID)),
			},
			"assign_current_droplet": {
				HREF:   prefixedLinkURL(baseURL, fmt.Sprintf("v3/apps/%s/relationships/current_droplet", dropletRecord.AppGUID)),
				Method: "PATCH",
			},
			"download": nil,
		},
	}
	if dropletRecord.DropletErrorMsg != "" {
		toReturn.Error = &dropletRecord.DropletErrorMsg
	}
	return toReturn
}
