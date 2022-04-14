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

func (c *BuildCreate) ToMessage(record repositories.PackageRecord) repositories.CreateBuildMessage {
	toReturn := repositories.CreateBuildMessage{
		AppGUID:         record.AppGUID,
		PackageGUID:     c.Package.GUID,
		SpaceGUID:       record.SpaceGUID,
		StagingMemoryMB: DefaultLifecycleConfig.StagingMemoryMB,
		StagingDiskMB:   DefaultLifecycleConfig.StagingDiskMB,
		Lifecycle: repositories.Lifecycle{
			Type: DefaultLifecycleConfig.Type,
			Data: repositories.LifecycleData{
				Buildpacks: []string{},
				Stack:      DefaultLifecycleConfig.Stack,
			},
		},
		Labels:      c.Metadata.Labels,
		Annotations: c.Metadata.Annotations,
		OwnerRef: metav1.OwnerReference{
			APIVersion: repositories.APIVersion,
			Kind:       "CFPackage",
			Name:       record.GUID,
			UID:        record.UID,
		},
	}

	return toReturn
}
