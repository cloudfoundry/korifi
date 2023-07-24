package payloads

import (
	"fmt"
	"net/url"
	"regexp"

	"code.cloudfoundry.org/korifi/api/config"
	"code.cloudfoundry.org/korifi/api/payloads/parse"
	"code.cloudfoundry.org/korifi/api/payloads/validation"
	"code.cloudfoundry.org/korifi/api/repositories"
	jellidation "github.com/jellydator/validation"
	"k8s.io/apimachinery/pkg/labels"
)

// DefaultLifecycleConfig is overwritten by main.go
var DefaultLifecycleConfig = config.DefaultLifecycleConfig{
	Type:            "buildpack",
	Stack:           "cflinuxfs3",
	StagingMemoryMB: 1024,
}

type AppCreate struct {
	Name                 string            `json:"name"`
	EnvironmentVariables map[string]string `json:"environment_variables"`
	Relationships        *AppRelationships `json:"relationships"`
	Lifecycle            *Lifecycle        `json:"lifecycle"`
	Metadata             Metadata          `json:"metadata"`
}

var appNameRegex = regexp.MustCompile(`^[-\w]+$`)

func (c AppCreate) Validate() error {
	return jellidation.ValidateStruct(&c,
		jellidation.Field(&c.Name, jellidation.Required, jellidation.Match(appNameRegex).Error("name must consist only of letters, numbers, underscores and dashes")),
		jellidation.Field(&c.Relationships, jellidation.NotNil),
		jellidation.Field(&c.Lifecycle),
		jellidation.Field(&c.Metadata),
	)
}

type AppRelationships struct {
	Space *Relationship `json:"space"`
}

func (r AppRelationships) Validate() error {
	return jellidation.ValidateStruct(&r,
		jellidation.Field(&r.Space, jellidation.NotNil),
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
	return jellidation.ValidateStruct(&d,
		jellidation.Field(&d.Relationship, jellidation.NotNil),
	)
}

type AppList struct {
	Names         string
	GUIDs         string
	SpaceGuids    string
	OrderBy       string
	LabelSelector labels.Selector
}

func (a AppList) Validate() error {
	return jellidation.ValidateStruct(&a,
		jellidation.Field(&a.OrderBy, validation.OneOfOrderBy("created_at", "updated_at", "name", "state")),
	)
}

func (a *AppList) ToMessage() repositories.ListAppsMessage {
	return repositories.ListAppsMessage{
		Names:         parse.ArrayParam(a.Names),
		Guids:         parse.ArrayParam(a.GUIDs),
		SpaceGuids:    parse.ArrayParam(a.SpaceGuids),
		LabelSelector: a.LabelSelector,
	}
}

func (a *AppList) SupportedKeys() []string {
	return []string{"names", "guids", "space_guids", "order_by", "per_page", "page", "label_selector"}
}

func (a *AppList) DecodeFromURLValues(values url.Values) error {
	a.Names = values.Get("names")
	a.GUIDs = values.Get("guids")
	a.SpaceGuids = values.Get("space_guids")
	a.OrderBy = values.Get("order_by")

	selector, err := labels.Parse(values.Get("label_selector"))
	if err != nil {
		return err
	}
	a.LabelSelector = selector

	return nil
}

type AppPatchEnvVars struct {
	Var map[string]interface{} `json:"var"`
}

func (p AppPatchEnvVars) Validate() error {
	return jellidation.ValidateStruct(&p,
		jellidation.Field(&p.Var,
			validation.StrictlyRequired,
			jellidation.Map().Keys(
				validation.NotStartWith("VCAP_"),
				validation.NotStartWith("VMC_"),
				validation.NotEqual("PORT"),
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
	Name      string          `json:"name"`
	Metadata  MetadataPatch   `json:"metadata"`
	Lifecycle *LifecyclePatch `json:"lifecycle"`
}

func (p AppPatch) Validate() error {
	return jellidation.ValidateStruct(&p,
		jellidation.Field(&p.Name, jellidation.Match(appNameRegex).Error("name must consist only of letters, numbers, underscores and dashes")),
		jellidation.Field(&p.Metadata),
		jellidation.Field(&p.Lifecycle),
	)
}

func (a *AppPatch) ToMessage(appGUID, spaceGUID string) repositories.PatchAppMessage {
	msg := repositories.PatchAppMessage{
		AppGUID:   appGUID,
		SpaceGUID: spaceGUID,
		Name:      a.Name,
		MetadataPatch: repositories.MetadataPatch{
			Annotations: a.Metadata.Annotations,
			Labels:      a.Metadata.Labels,
		},
	}

	if a.Lifecycle != nil {
		msg.Lifecycle = &repositories.LifecyclePatch{}

		if a.Lifecycle.Data != nil {
			msg.Lifecycle.Data = &repositories.LifecycleDataPatch{
				Stack:      a.Lifecycle.Data.Stack,
				Buildpacks: a.Lifecycle.Data.Buildpacks,
			}
		}
	}

	return msg
}
