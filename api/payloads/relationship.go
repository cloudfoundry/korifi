package payloads

import (
	payload_validation "code.cloudfoundry.org/korifi/api/payloads/validation"
	"github.com/jellydator/validation"
)

type Relationship struct {
	Data *RelationshipData `json:"data" validate:"required"`
}

func (r Relationship) Validate() error {
	return validation.ValidateStruct(&r, validation.Field(&r.Data, payload_validation.StrictlyRequired))
}

type RelationshipData struct {
	GUID string `json:"guid" validate:"required"`
}

func (r RelationshipData) Validate() error {
	return validation.ValidateStruct(&r, validation.Field(&r.GUID, payload_validation.StrictlyRequired))
}
