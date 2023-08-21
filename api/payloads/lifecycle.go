package payloads

import (
	"code.cloudfoundry.org/korifi/api/payloads/validation"
	jellidation "github.com/jellydator/validation"
)

type Lifecycle struct {
	Type string         `json:"type"`
	Data *LifecycleData `json:"data"`
}

func (l Lifecycle) Validate() error {
	return jellidation.ValidateStruct(&l,
		jellidation.Field(&l.Type, jellidation.Required),
		jellidation.Field(&l.Data, jellidation.NotNil),
	)
}

type LifecycleData struct {
	Buildpacks []string `json:"buildpacks"`
	Stack      string   `json:"stack"`
}

type LifecyclePatch struct {
	Type string              `json:"type"`
	Data *LifecycleDataPatch `json:"data"`
}

func (p LifecyclePatch) Validate() error {
	return jellidation.ValidateStruct(&p,
		jellidation.Field(&p.Type, validation.OneOf("buildpack")),
		jellidation.Field(&p.Data, jellidation.NotNil),
	)
}

type LifecycleDataPatch struct {
	Buildpacks *[]string `json:"buildpacks"`
	Stack      string    `json:"stack"`
}
