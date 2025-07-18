package payloads

import (
	"net/url"

	"code.cloudfoundry.org/korifi/api/repositories"
	jellidation "github.com/jellydator/validation"
)

type StackList struct {
	Pagination Pagination
}

func (s StackList) Validate() error {
	return jellidation.ValidateStruct(&s,
		jellidation.Field(&s.Pagination),
	)
}

func (s *StackList) ToMessage() (message repositories.ListStacksMessage) {
	return repositories.ListStacksMessage{
		Pagination: s.Pagination.ToMessage(DefaultPageSize),
	}
}

func (l *StackList) SupportedKeys() []string {
	return []string{"per_page", "page"}
}

func (l *StackList) DecodeFromURLValues(values url.Values) error {
	return l.Pagination.DecodeFromURLValues(values)
}
