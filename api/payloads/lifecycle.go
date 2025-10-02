package payloads

import (
	"fmt"

	"code.cloudfoundry.org/korifi/api/payloads/validation"
	jellidation "github.com/jellydator/validation"
)

var validLifecycleData lifeCycleDataRule

type lifeCycleDataRule struct{}

func (d lifeCycleDataRule) Validate(value any) error {
	data, ok := value.(LifecycleData)
	if !ok {
		return fmt.Errorf("%T is not supported, LifecycleData is expected", value)
	}
	return jellidation.ValidateStruct(&data,
		jellidation.Field(&data.Stack, jellidation.Required),
	)
}

type Lifecycle struct {
	Type string        `json:"type"`
	Data LifecycleData `json:"data"`
}

func (l Lifecycle) Validate() error {
	return jellidation.ValidateStruct(&l,
		jellidation.Field(&l.Type,
			jellidation.Required,
			validation.OneOf("buildpack", "docker")),
		jellidation.Field(&l.Data,
			jellidation.When(l.Type == "buildpack", jellidation.Required, validLifecycleData).
				Else(jellidation.Empty.Error("must be an empty object")),
		),
	)
}

type LifecycleData struct {
	Buildpacks []string `json:"buildpacks,omitempty"`
	Stack      string   `json:"stack,omitempty"`
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
