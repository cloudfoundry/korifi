package presenter

import (
	"net/url"

	"code.cloudfoundry.org/korifi/api/repositories"
	"code.cloudfoundry.org/korifi/api/repositories/include"
	"code.cloudfoundry.org/korifi/tools"
)

const (
	packagesBase = "/v3/packages"
)

type PackageResponse struct {
	GUID          string                       `json:"guid"`
	Type          string                       `json:"type"`
	Data          PackageData                  `json:"data"`
	State         string                       `json:"state"`
	Relationships map[string]ToOneRelationship `json:"relationships"`
	Links         PackageLinks                 `json:"links"`
	Metadata      Metadata                     `json:"metadata"`
	CreatedAt     string                       `json:"created_at"`
	UpdatedAt     string                       `json:"updated_at"`
}

type PackageData struct {
	Image string `json:"image,omitempty"`
}

type PackageLinks struct {
	Self     Link `json:"self"`
	Upload   Link `json:"upload"`
	Download Link `json:"download"`
	App      Link `json:"app"`
}

func ForPackage(record repositories.PackageRecord, baseURL url.URL, includes ...include.Resource) PackageResponse {
	return PackageResponse{
		GUID:          record.GUID,
		Type:          record.Type,
		State:         record.State,
		CreatedAt:     tools.ZeroIfNil(formatTimestamp(&record.CreatedAt)),
		UpdatedAt:     tools.ZeroIfNil(formatTimestamp(record.UpdatedAt)),
		Relationships: ForRelationships(record.Relationships()),
		Links: PackageLinks{
			Self: Link{
				HRef: buildURL(baseURL).appendPath(packagesBase, record.GUID).build(),
			},
			Upload: Link{
				HRef:   buildURL(baseURL).appendPath(packagesBase, record.GUID, "upload").build(),
				Method: "POST",
			},
			Download: Link{
				HRef:   buildURL(baseURL).appendPath(packagesBase, record.GUID, "download").build(),
				Method: "GET",
			},
			App: Link{
				HRef: buildURL(baseURL).appendPath(appsBase, record.AppGUID).build(),
			},
		},
		Metadata: Metadata{
			Labels:      emptyMapIfNil(record.Labels),
			Annotations: emptyMapIfNil(record.Annotations),
		},
		Data: PackageData{
			Image: record.ImageRef,
		},
	}
}
