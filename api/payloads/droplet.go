package payloads

import (
	"net/url"

	"code.cloudfoundry.org/korifi/api/payloads/parse"
	"code.cloudfoundry.org/korifi/api/payloads/validation"
	"code.cloudfoundry.org/korifi/api/repositories"
	jellidation "github.com/jellydator/validation"
)

type DropletUpdate struct {
	Metadata MetadataPatch `json:"metadata"`
}

func (d DropletUpdate) Validate() error {
	return jellidation.ValidateStruct(&d,
		jellidation.Field(&d.Metadata),
	)
}

func (c *DropletUpdate) ToMessage(dropletGUID string) repositories.UpdateDropletMessage {
	return repositories.UpdateDropletMessage{
		GUID: dropletGUID,
		MetadataPatch: repositories.MetadataPatch{
			Labels:      c.Metadata.Labels,
			Annotations: c.Metadata.Annotations,
		},
	}
}

type DropletList struct {
	GUIDs      string
	AppGUIDs   string
	SpaceGUIDs string
	OrderBy    string
	Pagination Pagination
}

func (l *DropletList) SupportedKeys() []string {
	return []string{
		"guids",
		"states",
		"app_guids",
		"space_guids",
		"organization_guids",
		"order_by",
		"page",
		"per_page",
	}
}

func (l DropletList) Validate() error {
	return jellidation.ValidateStruct(&l,
		jellidation.Field(&l.OrderBy, validation.OneOfOrderBy("created_at", "updated_at")),
		jellidation.Field(&l.Pagination),
	)
}

func (l *DropletList) DecodeFromURLValues(values url.Values) error {
	l.GUIDs = values.Get("guids")
	l.AppGUIDs = values.Get("app_guids")
	l.SpaceGUIDs = values.Get("space_guids")
	l.OrderBy = values.Get("order_by")
	return l.Pagination.DecodeFromURLValues(values)
}

func (l *DropletList) ToMessage() repositories.ListDropletsMessage {
	return repositories.ListDropletsMessage{
		GUIDs:      parse.ArrayParam(l.GUIDs),
		AppGUIDs:   parse.ArrayParam(l.AppGUIDs),
		SpaceGUIDs: parse.ArrayParam(l.SpaceGUIDs),
		OrderBy:    l.OrderBy,
		Pagination: l.Pagination.ToMessage(DefaultPageSize),
	}
}
