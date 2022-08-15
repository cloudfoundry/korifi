package payloads

import (
	"fmt"

	"code.cloudfoundry.org/korifi/api/config"
	"code.cloudfoundry.org/korifi/api/repositories"
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

func (p AppCreate) ToAppCreateMessage() repositories.CreateAppMessage {
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

	return repositories.CreateAppMessage{
		Name:                 p.Name,
		SpaceGUID:            p.Relationships.Space.Data.GUID,
		Labels:               p.Metadata.Labels,
		Annotations:          p.Metadata.Annotations,
		State:                repositories.StoppedState,
		Lifecycle:            lifecycleBlock,
		EnvironmentVariables: p.EnvironmentVariables,
	}
}

type AppSetCurrentDroplet struct {
	Relationship `json:",inline" validate:"required"`
}

type AppList struct {
	Names      *string `schema:"names"`
	GUIDs      *string `schema:"guids"`
	SpaceGuids *string `schema:"space_guids"`
	OrderBy    string  `schema:"order_by"`
}

func (a *AppList) ToMessage() repositories.ListAppsMessage {
	return repositories.ListAppsMessage{
		Names:      ParseArrayParam(a.Names),
		Guids:      ParseArrayParam(a.GUIDs),
		SpaceGuids: ParseArrayParam(a.SpaceGuids),
	}
}

func (a *AppList) SupportedKeys() []string {
	return []string{"names", "guids", "space_guids", "order_by"}
}

type AppPatchEnvVars struct {
	Var map[string]interface{} `json:"var" validate:"required,dive,keys,startsnotwith=VCAP_,startsnotwith=VMC_,ne=PORT,endkeys"`
}

func (a *AppPatchEnvVars) ToMessage(appGUID, spaceGUID string) repositories.PatchAppEnvVarsMessage {
	message := repositories.PatchAppEnvVarsMessage{
		AppGUID:              appGUID,
		SpaceGUID:            spaceGUID,
		EnvironmentVariables: map[string]*string{},
	}

	for k, v := range a.Var {
		switch v := v.(type) {
		case nil:
			message.EnvironmentVariables[k] = nil
		case bool:
			stringVar := fmt.Sprintf("%t", v)
			message.EnvironmentVariables[k] = &stringVar
		case float32:
			stringVar := fmt.Sprintf("%f", v)
			message.EnvironmentVariables[k] = &stringVar
		case int:
			stringVar := fmt.Sprintf("%d", v)
			message.EnvironmentVariables[k] = &stringVar
		case string:
			message.EnvironmentVariables[k] = &v
		}
	}

	return message
}

type AppPatch struct {
	Metadata MetadataPatch `json:"metadata"`
}

func (a *AppPatch) ToMessage(appGUID, spaceGUID string) repositories.PatchAppMetadataMessage {
	return repositories.PatchAppMetadataMessage{
		AppGUID:     appGUID,
		SpaceGUID:   spaceGUID,
		Annotations: a.Metadata.Annotations,
		Labels:      a.Metadata.Labels,
	}
}
