package payloads

import "code.cloudfoundry.org/cf-k8s-controllers/api/repositories"

type BuildCreate struct {
	Package         *RelationshipData `json:"package" validate:"required"`
	StagingMemoryMB *int              `json:"staging_memory_in_mb"`
	StagingDiskMB   *int              `json:"staging_disk_in_mb"`
	Lifecycle       *Lifecycle        `json:"lifecycle"`
	Metadata        Metadata          `json:"metadata"`
}

func (c *BuildCreate) ToMessage(appGUID string, spaceGUID string) repositories.BuildCreateMessage {
	toReturn := repositories.BuildCreateMessage{
		AppGUID:         appGUID,
		PackageGUID:     c.Package.GUID,
		SpaceGUID:       spaceGUID,
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
	}

	return toReturn
}
