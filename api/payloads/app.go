package payloads

import (
	"code.cloudfoundry.org/cf-k8s-controllers/api/config"
	"code.cloudfoundry.org/cf-k8s-controllers/api/repositories"
)

// DefaultLifecycleConfig is overwritten by main.go
var DefaultLifecycleConfig = config.DefaultLifecycleConfig{
	Type:            "buildpack",
	Stack:           "cflinuxfs3",
	StagingMemoryMB: 1024,
	StagingDiskMB:   1024,
}

type AppCreate struct {
	Name                 string            `json:"name" validate:"required"`
	EnvironmentVariables map[string]string `json:"environment_variables"`
	Relationships        AppRelationships  `json:"relationships" validate:"required"`
	Lifecycle            *Lifecycle        `json:"lifecycle"`
	Metadata             Metadata          `json:"metadata"`
}

type AppRelationships struct {
	Space Relationship `json:"space" validate:"required"`
}

func (p AppCreate) ToAppCreateMessage() repositories.AppCreateMessage {
	lifecycleBlock := repositories.Lifecycle{
		Type: DefaultLifecycleConfig.Type,
		Data: repositories.LifecycleData{
			Stack: DefaultLifecycleConfig.Stack,
		},
	}
	if p.Lifecycle != nil {
		lifecycleBlock.Data.Stack = p.Lifecycle.Data.Stack
		lifecycleBlock.Data.Buildpacks = p.Lifecycle.Data.Buildpacks
	}

	return repositories.AppCreateMessage{
		Name:        p.Name,
		SpaceGUID:   p.Relationships.Space.Data.GUID,
		Labels:      p.Metadata.Labels,
		Annotations: p.Metadata.Annotations,
		State:       repositories.StoppedState,
		Lifecycle:   lifecycleBlock,
		HasEnvVars:  len(p.EnvironmentVariables) > 0,
	}
}

type AppSetCurrentDroplet struct {
	Relationship `json:",inline" validate:"required"`
}
