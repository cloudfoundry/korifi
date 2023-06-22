package payloads

import (
	"net/url"

	"code.cloudfoundry.org/korifi/api/payloads/validation"
	jellidation "github.com/jellydator/validation"
)

type BuildpackList struct {
	OrderBy string
}

func (d BuildpackList) SupportedKeys() []string {
	return []string{"order_by", "per_page", "page"}
}

func (d *BuildpackList) DecodeFromURLValues(values url.Values) error {
	d.OrderBy = values.Get("order_by")
	return nil
}

func (d BuildpackList) Validate() error {
	validOrderBys := []string{"created_at", "updated_at", "position"}
	var allowed []any
	for _, a := range validOrderBys {
		allowed = append(allowed, a, "-"+a)
	}

	return jellidation.ValidateStruct(&d,
		jellidation.Field(&d.OrderBy, validation.OneOf(allowed...)),
	)
}
