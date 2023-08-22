package payloads

import (
	"fmt"

	"code.cloudfoundry.org/korifi/api/payloads/validation"
	jellidation "github.com/jellydator/validation"
)

type Lifecycle struct {
	Type string         `json:"type"`
	Data *LifecycleData `json:"data"`
}

func (l Lifecycle) Validate() error {
	lifecycleDataRule := jellidation.By(func(value any) error {
		data, ok := value.(*LifecycleData)
		if !ok {
			return fmt.Errorf("%T is not supported, LifecycleData is expected", value)
		}

		if l.Type == "docker" {
			return data.ValidateDockerLifecycleData()
		}

		if l.Type == "buildpack" {
			return data.ValidateBuildpackLifecycleData()
		}

		return nil
	})

	return jellidation.ValidateStruct(&l,
		jellidation.Field(&l.Type,
			jellidation.Required,
			validation.OneOf("buildpack", "docker")),
		jellidation.Field(&l.Data, jellidation.Required, lifecycleDataRule),
	)
}

type LifecycleData struct {
	Buildpacks []string `json:"buildpacks,omitempty"`
	Stack      string   `json:"stack,omitempty"`
}

func (d LifecycleData) ValidateBuildpackLifecycleData() error {
	return jellidation.ValidateStruct(&d,
		jellidation.Field(&d.Stack, jellidation.Required),
	)
}

func (d LifecycleData) ValidateDockerLifecycleData() error {
	return jellidation.Validate(&d, jellidation.In(LifecycleData{}).Error("must be an empty object"))
}

type LifecyclePatch struct {
	Type string              `json:"type"`
	Data *LifecycleDataPatch `json:"data"`
}

func (p LifecyclePatch) Validate() error {
	return jellidation.ValidateStruct(&p,
		jellidation.Field(&p.Type, validation.OneOf("buildpack", "docker")),
		jellidation.Field(&p.Data, jellidation.NotNil),
	)
}

type LifecycleDataPatch struct {
	Buildpacks *[]string `json:"buildpacks"`
	Stack      string    `json:"stack"`
}
