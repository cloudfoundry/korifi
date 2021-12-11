package presenter

import (
	"net/url"

	"code.cloudfoundry.org/cf-k8s-controllers/api/repositories"
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

func ForDroplet(dropletRecord repositories.DropletRecord, baseURL url.URL) DropletResponse {
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
				Data: &RelationshipData{
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
				HREF: buildURL(baseURL).appendPath(dropletsBase, dropletRecord.GUID).build(),
			},
			"package": {
				HREF: buildURL(baseURL).appendPath(packagesBase, dropletRecord.PackageGUID).build(),
			},
			"app": {
				HREF: buildURL(baseURL).appendPath(appsBase, dropletRecord.AppGUID).build(),
			},
			"assign_current_droplet": {
				HREF:   buildURL(baseURL).appendPath(appsBase, dropletRecord.AppGUID, "relationships/current_droplet").build(),
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

func ForDropletList(dropletRecordList []repositories.DropletRecord, baseURL, requestURL url.URL) ListResponse {
	dropletResponses := make([]interface{}, 0, len(dropletRecordList))
	for _, droplet := range dropletRecordList {
		dropletResponses = append(dropletResponses, ForDroplet(droplet, baseURL))
	}
	// https://v3-apidocs.cloudfoundry.org/version/3.100.0/index.html#list-droplets-for-a-package
	// https://api.example.org/v3/packages/7b34f1cf-7e73-428a-bb5a-8a17a8058396/droplets
	return ForList(dropletResponses, baseURL, requestURL)
}
