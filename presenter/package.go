package presenter

import (
	"fmt"

	"code.cloudfoundry.org/cf-k8s-api/repositories"
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

type PackageData struct {
}

type PackageLinks struct {
	Self     Link `json:"self"`
	Upload   Link `json:"upload"`
	Download Link `json:"download"`
	App      Link `json:"app"`
}

func ForPackage(record repositories.PackageRecord, baseURL string) PackageResponse {
	return PackageResponse{
		GUID:      record.GUID,
		Type:      record.Type,
		State:     record.State,
		CreatedAt: record.CreatedAt,
		UpdatedAt: record.UpdatedAt,
		Relationships: Relationships{
			"app": Relationship{
				Data: RelationshipData{
					GUID: record.AppGUID,
				},
			},
		},
		Links: PackageLinks{
			Self: Link{
				HREF: prefixedLinkURL(baseURL, fmt.Sprintf("v3/packages/%s", record.GUID)),
			},
			Upload: Link{
				HREF:   prefixedLinkURL(baseURL, fmt.Sprintf("v3/packages/%s/upload", record.GUID)),
				Method: "POST",
			},
			Download: Link{
				HREF:   prefixedLinkURL(baseURL, fmt.Sprintf("v3/packages/%s/download", record.GUID)),
				Method: "GET",
			},
			App: Link{
				HREF: prefixedLinkURL(baseURL, fmt.Sprintf("v3/apps/%s", record.AppGUID)),
			},
		},
		Metadata: Metadata{
			Labels:      map[string]string{},
			Annotations: map[string]string{},
		},
	}
}
