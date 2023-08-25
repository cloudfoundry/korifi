package payloads

import (
	payload_validation "code.cloudfoundry.org/korifi/api/payloads/validation"
	"code.cloudfoundry.org/korifi/api/repositories"
	"github.com/jellydator/validation"
)

type BuildCreate struct {
	Package         *RelationshipData `json:"package"`
	StagingMemoryMB *int              `json:"staging_memory_in_mb"`
	StagingDiskMB   *int              `json:"staging_disk_in_mb"`
	Lifecycle       *Lifecycle        `json:"lifecycle"`
	Metadata        BuildMetadata     `json:"metadata"`
}

func (b BuildCreate) Validate() error {
	return validation.ValidateStruct(&b,
		validation.Field(&b.Package, payload_validation.StrictlyRequired),
		validation.Field(&b.Metadata),
		validation.Field(&b.Lifecycle),
	)
}

func (c *BuildCreate) ToMessage(appRecord repositories.AppRecord) repositories.CreateBuildMessage {
	toReturn := repositories.CreateBuildMessage{
		AppGUID:         appRecord.GUID,
		PackageGUID:     c.Package.GUID,
		SpaceGUID:       appRecord.SpaceGUID,
		StagingMemoryMB: DefaultLifecycleConfig.StagingMemoryMB,
		Lifecycle:       appRecord.Lifecycle,
		Labels:          c.Metadata.Labels,
		Annotations:     c.Metadata.Annotations,
	}

	return toReturn
}
