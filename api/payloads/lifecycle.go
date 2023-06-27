package payloads

import (
	"github.com/jellydator/validation"
)

type Lifecycle struct {
	Type string         `json:"type"`
	Data *LifecycleData `json:"data"`
}

func (l Lifecycle) Validate() error {
	return validation.ValidateStruct(&l,
		validation.Field(&l.Type, validation.Required),
		validation.Field(&l.Data, validation.NotNil),
	)
}

type LifecycleData struct {
	Buildpacks []string `json:"buildpacks"`
	Stack      string   `json:"stack"`
}

func (d LifecycleData) Validate() error {
	return validation.ValidateStruct(&d,
		validation.Field(&d.Stack, validation.Required),
	)
}
