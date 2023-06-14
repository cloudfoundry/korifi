package payloads

import (
	payload_validation "code.cloudfoundry.org/korifi/api/payloads/validation"
	"github.com/jellydator/validation"
)

type Lifecycle struct {
	Type string        `json:"type" validate:"required"`
	Data LifecycleData `json:"data" validate:"required"`
}

func (l Lifecycle) Validate() error {
	return validation.ValidateStruct(&l,
		validation.Field(&l.Type, payload_validation.StrictlyRequired),
		validation.Field(&l.Data, payload_validation.StrictlyRequired),
	)
}

type LifecycleData struct {
	Buildpacks []string `json:"buildpacks" validate:"required"`
	Stack      string   `json:"stack" validate:"required"`
}

func (d LifecycleData) Validate() error {
	return validation.ValidateStruct(&d,
		validation.Field(&d.Buildpacks, payload_validation.StrictlyRequired),
		validation.Field(&d.Stack, payload_validation.StrictlyRequired),
	)
}
