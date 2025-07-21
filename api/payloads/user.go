package payloads

import (
	"net/url"

	"code.cloudfoundry.org/korifi/api/payloads/parse"
	"code.cloudfoundry.org/korifi/api/repositories"
	jellidation "github.com/jellydator/validation"
)

type UserList struct {
	Names      string
	Pagination Pagination
}

func (l UserList) Validate() error {
	return jellidation.ValidateStruct(&l,
		jellidation.Field(&l.Pagination),
	)
}

func (l *UserList) ToMessage() (message repositories.ListUsersMessage) {
	return repositories.ListUsersMessage{
		Names:      parse.ArrayParam(l.Names),
		Pagination: l.Pagination.ToMessage(DefaultPageSize),
	}
}

func (l *UserList) SupportedKeys() []string {
	return []string{"per_page", "page", "usernames"}
}

func (l *UserList) DecodeFromURLValues(values url.Values) error {
	l.Names = values.Get("usernames")
	return l.Pagination.DecodeFromURLValues(values)
}
