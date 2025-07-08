package payloads

import (
	"net/url"

	"code.cloudfoundry.org/korifi/api/payloads/validation"
	"code.cloudfoundry.org/korifi/api/repositories"
	jellidation "github.com/jellydator/validation"
)

type BuildpackList struct {
	OrderBy    string
	Pagination Pagination
}

func (b BuildpackList) ToMessage() repositories.ListBuildpacksMessage {
	return repositories.ListBuildpacksMessage{
		OrderBy:    b.OrderBy,
		Pagination: b.Pagination.ToMessage(DefaultPageSize),
	}
}

func (d BuildpackList) SupportedKeys() []string {
	return []string{"order_by", "per_page", "page"}
}

func (d *BuildpackList) DecodeFromURLValues(values url.Values) error {
	d.OrderBy = values.Get("order_by")
	return d.Pagination.DecodeFromURLValues(values)
}

func (d BuildpackList) Validate() error {
	return jellidation.ValidateStruct(&d,
		jellidation.Field(&d.Pagination),
		jellidation.Field(&d.OrderBy, validation.OneOfOrderBy("created_at", "updated_at", "position")),
	)
}
