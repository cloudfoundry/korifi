package payloads

import (
	"net/url"

	"code.cloudfoundry.org/korifi/api/payloads/parse"
	payload_validation "code.cloudfoundry.org/korifi/api/payloads/validation"
	"code.cloudfoundry.org/korifi/api/repositories"
	jellidation "github.com/jellydator/validation"
)

type BuildCreate struct {
	Package         *RelationshipData `json:"package"`
	StagingMemoryMB *int              `json:"staging_memory_in_mb"`
	StagingDiskMB   *int              `json:"staging_disk_in_mb"`
	Lifecycle       *Lifecycle        `json:"lifecycle"`
	Metadata        BuildMetadata     `json:"metadata"`
}

func (b BuildCreate) Validate() error {
	return jellidation.ValidateStruct(&b,
		jellidation.Field(&b.Package, payload_validation.StrictlyRequired),
		jellidation.Field(&b.Metadata),
		jellidation.Field(&b.Lifecycle),
	)
}

func (c *BuildCreate) ToMessage(appRecord repositories.AppRecord) repositories.CreateBuildMessage {
	lifecycle := appRecord.Lifecycle
	if c.Lifecycle != nil {
		lifecycle = repositories.Lifecycle{
			Type: c.Lifecycle.Type,
			Data: repositories.LifecycleData{
				Buildpacks: c.Lifecycle.Data.Buildpacks,
				Stack:      c.Lifecycle.Data.Stack,
			},
		}
	}

	toReturn := repositories.CreateBuildMessage{
		AppGUID:         appRecord.GUID,
		PackageGUID:     c.Package.GUID,
		SpaceGUID:       appRecord.SpaceGUID,
		StagingMemoryMB: DefaultLifecycleConfig.StagingMemoryMB,
		Lifecycle:       lifecycle,
		Labels:          c.Metadata.Labels,
		Annotations:     c.Metadata.Annotations,
	}

	return toReturn
}

type BuildList struct {
	PackageGUIDs string
	AppGUIDs     string
	States       string
	OrderBy      string
	Pagination   Pagination
}

func (b *BuildList) ToMessage() repositories.ListBuildsMessage {
	return repositories.ListBuildsMessage{
		PackageGUIDs: parse.ArrayParam(b.PackageGUIDs),
		AppGUIDs:     parse.ArrayParam(b.AppGUIDs),
		States:       parse.ArrayParam(b.States),
		OrderBy:      b.OrderBy,
		Pagination:   b.Pagination.ToMessage(DefaultPageSize),
	}
}

func (b *BuildList) SupportedKeys() []string {
	return []string{"package_guids", "app_guids", "states", "order_by", "per_page", "page"}
}

func (b *BuildList) DecodeFromURLValues(values url.Values) error {
	b.PackageGUIDs = values.Get("package_guids")
	b.AppGUIDs = values.Get("app_guids")
	b.States = values.Get("states")
	b.OrderBy = values.Get("order_by")
	return b.Pagination.DecodeFromURLValues(values)
}

func (b BuildList) Validate() error {
	return jellidation.ValidateStruct(&b,
		jellidation.Field(&b.OrderBy, payload_validation.OneOfOrderBy("created_at", "updated_at")),
		jellidation.Field(&b.Pagination),
	)
}
