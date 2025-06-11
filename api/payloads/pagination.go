package payloads

import (
	"net/url"

	"code.cloudfoundry.org/korifi/api/payloads/parse"
	"code.cloudfoundry.org/korifi/api/payloads/validation"
	"code.cloudfoundry.org/korifi/api/repositories"
	"code.cloudfoundry.org/korifi/tools"
	jellidation "github.com/jellydator/validation"
)

type Pagination struct {
	PerPage string
	Page    string
}

func (p Pagination) Validate() error {
	return jellidation.ValidateStruct(&p,
		jellidation.Field(&p.PerPage, jellidation.When(p.PerPage != "",
			validation.NotEqual("0"), jellidation.By(validation.IntegerMatching(jellidation.Min(1), jellidation.Max(5000)))),
		),
		jellidation.Field(&p.Page, jellidation.When(p.Page != "",
			validation.NotEqual("0"), jellidation.By(validation.IntegerMatching(jellidation.Min(1)))),
		),
	)
}

func (p *Pagination) DecodeFromURLValues(values url.Values) error {
	p.PerPage = values.Get("per_page")
	p.Page = values.Get("page")
	return nil
}

func (p *Pagination) ToMessage(defaultPageSize int) repositories.Pagination {
	return repositories.Pagination{
		Page:    tools.IfZero(parse.Integer(p.Page), 1),
		PerPage: tools.IfZero(parse.Integer(p.PerPage), defaultPageSize),
	}
}
