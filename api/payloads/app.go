package payloads

import (
	"fmt"
	"net/url"

	"code.cloudfoundry.org/korifi/api/config"
	"code.cloudfoundry.org/korifi/api/payloads/parse"
	payload_validation "code.cloudfoundry.org/korifi/api/payloads/validation"
	"code.cloudfoundry.org/korifi/api/repositories"
	"github.com/jellydator/validation"
)

// DefaultLifecycleConfig is overwritten by main.go
var DefaultLifecycleConfig = config.DefaultLifecycleConfig{
	Type:            "buildpack",
	Stack:           "cflinuxfs3",
	StagingMemoryMB: 1024,
	StagingDiskMB:   1024,
}

type AppCreate struct {
	Name                 string            `json:"name"`
	EnvironmentVariables map[string]string `json:"environment_variables"`
	Relationships        AppRelationships  `json:"relationships"`
	Lifecycle            *Lifecycle        `json:"lifecycle"`
	Metadata             Metadata          `json:"metadata"`
}

func (c AppCreate) Validate() error {
	return validation.ValidateStruct(&c,
		validation.Field(&c.Name, payload_validation.StrictlyRequired),
		validation.Field(&c.Relationships, payload_validation.StrictlyRequired),
		validation.Field(&c.Lifecycle),
		validation.Field(&c.Metadata),
	)
}

type AppRelationships struct {
	Space Relationship `json:"space"`
}

func (r AppRelationships) Validate() error {
	return validation.ValidateStruct(&r,
		validation.Field(&r.Space, payload_validation.StrictlyRequired),
	)
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
		Metadata:             repositories.Metadata(p.Metadata),
		State:                repositories.StoppedState,
		Lifecycle:            lifecycleBlock,
		EnvironmentVariables: p.EnvironmentVariables,
	}
}

type AppSetCurrentDroplet struct {
	Relationship `json:",inline"`
}

func (d AppSetCurrentDroplet) Validate() error {
	return validation.ValidateStruct(&d,
		validation.Field(&d.Relationship, payload_validation.StrictlyRequired),
	)
}

type AppList struct {
	Names      string
	GUIDs      string
	SpaceGuids string
}

func (a *AppList) ToMessage() repositories.ListAppsMessage {
	return repositories.ListAppsMessage{
		Names:      parse.ArrayParam(a.Names),
		Guids:      parse.ArrayParam(a.GUIDs),
		SpaceGuids: parse.ArrayParam(a.SpaceGuids),
	}
}

func (a *AppList) SupportedKeys() []string {
	return []string{"names", "guids", "space_guids", "order_by", "per_page", "page"}
}

func (a *AppList) DecodeFromURLValues(values url.Values) error {
	a.Names = values.Get("names")
	a.GUIDs = values.Get("guids")
	a.SpaceGuids = values.Get("space_guids")
	return nil
}

type AppPatchEnvVars struct {
	Var map[string]interface{} `json:"var"`
}

func (p AppPatchEnvVars) Validate() error {
	return validation.ValidateStruct(&p,
		validation.Field(&p.Var,
			payload_validation.StrictlyRequired,
			validation.Map().Keys(
				payload_validation.NotStartWith("VCAP_"),
				payload_validation.NotStartWith("VMC_"),
				payload_validation.NotEqual("PORT"),
			).AllowExtraKeys(),
		))
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
		AppGUID:   appGUID,
		SpaceGUID: spaceGUID,
		MetadataPatch: repositories.MetadataPatch{
			Annotations: a.Metadata.Annotations,
			Labels:      a.Metadata.Labels,
		},
	}
}
