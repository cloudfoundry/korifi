package presenter

import (
	"net/url"

	"code.cloudfoundry.org/cf-k8s-controllers/api/repositories"
)

const (
	packagesBase = "/v3/packages"
)

type PackageResponse struct {
	GUID          string        `json:"guid"`
	Type          string        `json:"type"`
	Data          PackageData   `json:"data"`
	State         string        `json:"state"`
	Relationships Relationships `json:"relationships"`
	Links         PackageLinks  `json:"links"`
	Metadata      Metadata      `json:"metadata"`
	CreatedAt     string        `json:"created_at"`
	UpdatedAt     string        `json:"updated_at"`
}

type PackageListResponse struct {
	PaginationData PaginationData    `json:"pagination"`
	Resources      []PackageResponse `json:"resources"`
}

type PackageData struct{}

type PackageLinks struct {
	Self     Link `json:"self"`
	Upload   Link `json:"upload"`
	Download Link `json:"download"`
	App      Link `json:"app"`
}

func ForPackage(record repositories.PackageRecord, baseURL url.URL) PackageResponse {
	return PackageResponse{
		GUID:      record.GUID,
		Type:      record.Type,
		State:     record.State,
		CreatedAt: record.CreatedAt,
		UpdatedAt: record.UpdatedAt,
		Relationships: Relationships{
			"app": Relationship{
				Data: &RelationshipData{
					GUID: record.AppGUID,
				},
			},
		},
		Links: PackageLinks{
			Self: Link{
				HREF: buildURL(baseURL).appendPath(packagesBase, record.GUID).build(),
			},
			Upload: Link{
				HREF:   buildURL(baseURL).appendPath(packagesBase, record.GUID, "upload").build(),
				Method: "POST",
			},
			Download: Link{
				HREF:   buildURL(baseURL).appendPath(packagesBase, record.GUID, "download").build(),
				Method: "GET",
			},
			App: Link{
				HREF: buildURL(baseURL).appendPath(appsBase, record.AppGUID).build(),
			},
		},
		Metadata: Metadata{
			Labels:      map[string]string{},
			Annotations: map[string]string{},
		},
	}
}

func ForPackageList(packageRecordList []repositories.PackageRecord, baseURL url.URL) PackageListResponse {
	packageResponses := make([]PackageResponse, 0, len(packageRecordList))
	for _, currentPackage := range packageRecordList {
		packageResponses = append(packageResponses, ForPackage(currentPackage, baseURL))
	}

	packageListResponse := PackageListResponse{
		PaginationData: PaginationData{
			TotalResults: len(packageResponses),
			TotalPages:   1,
			First: PageRef{
				HREF: buildURL(baseURL).appendPath(packagesBase).build(),
			},
			Last: PageRef{
				HREF: buildURL(baseURL).appendPath(packagesBase).build(),
			},
		},
		Resources: packageResponses,
	}

	return packageListResponse
}
