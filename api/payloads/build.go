package payloads

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"code.cloudfoundry.org/korifi/api/repositories"
)

type BuildCreate struct {
	Package         *RelationshipData `json:"package" validate:"required"`
	StagingMemoryMB *int              `json:"staging_memory_in_mb"`
	StagingDiskMB   *int              `json:"staging_disk_in_mb"`
	Lifecycle       *Lifecycle        `json:"lifecycle"`
	Metadata        Metadata          `json:"metadata"`
}

func (c *BuildCreate) ToMessage(appRecord repositories.AppRecord, packageRecord repositories.PackageRecord) repositories.CreateBuildMessage {
	toReturn := repositories.CreateBuildMessage{
		AppGUID:         appRecord.GUID,
		PackageGUID:     c.Package.GUID,
		SpaceGUID:       appRecord.SpaceGUID,
		StagingMemoryMB: DefaultLifecycleConfig.StagingMemoryMB,
		StagingDiskMB:   DefaultLifecycleConfig.StagingDiskMB,
		Lifecycle:       appRecord.Lifecycle,
		Labels:          c.Metadata.Labels,
		Annotations:     c.Metadata.Annotations,
		OwnerRef: metav1.OwnerReference{
			APIVersion: repositories.APIVersion,
			Kind:       "CFPackage",
			Name:       packageRecord.GUID,
			UID:        packageRecord.UID,
		},
	}

	return toReturn
}
