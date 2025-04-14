package payloads

import (
	"github.com/jellydator/validation"
)

type Relationship struct {
	Data *RelationshipData `json:"data"`
}

func (r Relationship) Validate() error {
	return validation.ValidateStruct(&r,
		validation.Field(&r.Data, validation.NotNil),
	)
}

type RelationshipData struct {
	GUID string `json:"guid"`
}

func (r RelationshipData) Validate() error {
	return validation.ValidateStruct(&r,
		validation.Field(&r.GUID, validation.Required),
	)
}

type ToManyRelationship struct {
	Data []RelationshipData `json:"data"`
}
