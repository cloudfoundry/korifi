package message

import (
	"code.cloudfoundry.org/cf-k8s-api/repositories"
)

// TODO: Make these configurable
var (
	defaultLifecycleType  = "buildpack"
	defaultLifecycleStack = "cflinuxfs3"
)

type AppCreateMessage struct {
	Name                 string            `json:"name" validate:"required"`
	EnvironmentVariables map[string]string `json:"environment_variables"`
	Relationships        Relationship      `json:"relationships" validate:"required"`
	Lifecycle            *Lifecycle        `json:"lifecycle"`
	Metadata             Metadata          `json:"metadata"`
}

func AppCreateMessageToAppRecord(requestApp AppCreateMessage) repositories.AppRecord {
	lifecycleBlock := repositories.Lifecycle{
		Type: defaultLifecycleType,
		Data: repositories.LifecycleData{
			Stack: defaultLifecycleStack,
		},
	}
	if requestApp.Lifecycle != nil {
		lifecycleBlock.Data.Stack = requestApp.Lifecycle.Data.Stack
		lifecycleBlock.Data.Buildpacks = requestApp.Lifecycle.Data.Buildpacks
	}

	return repositories.AppRecord{
		Name:        requestApp.Name,
		GUID:        "",
		SpaceGUID:   requestApp.Relationships.Space.Data.GUID,
		Labels:      requestApp.Metadata.Labels,
		Annotations: requestApp.Metadata.Annotations,
		State:       repositories.StoppedState,
		Lifecycle:   lifecycleBlock,
		CreatedAt:   "",
		UpdatedAt:   "",
	}
}
