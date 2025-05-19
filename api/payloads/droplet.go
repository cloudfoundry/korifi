package payloads

import (
	"net/url"
	"regexp"

	"code.cloudfoundry.org/korifi/api/payloads/parse"
	"code.cloudfoundry.org/korifi/api/repositories"
	"github.com/jellydator/validation"
)

type DropletUpdate struct {
	Metadata MetadataPatch `json:"metadata"`
}

func (d DropletUpdate) Validate() error {
	return validation.ValidateStruct(&d,
		validation.Field(&d.Metadata),
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
}

func (l *DropletList) SupportedKeys() []string {
	return []string{
		"guids",
		"states",
		"app_guids",
		"space_guids",
		"organization_guids",
	}
}

func (l *DropletList) IgnoredKeys() []*regexp.Regexp {
	return []*regexp.Regexp{
		regexp.MustCompile("page"),
		regexp.MustCompile("per_page"),
		regexp.MustCompile("order_by"),
	}
}

func (l *DropletList) DecodeFromURLValues(values url.Values) error {
	l.GUIDs = values.Get("guids")
	l.AppGUIDs = values.Get("app_guids")
	l.SpaceGUIDs = values.Get("space_guids")
	return nil
}

func (l *DropletList) ToMessage() repositories.ListDropletsMessage {
	return repositories.ListDropletsMessage{
		GUIDs:      parse.ArrayParam(l.GUIDs),
		AppGUIDs:   parse.ArrayParam(l.AppGUIDs),
		SpaceGUIDs: parse.ArrayParam(l.SpaceGUIDs),
	}
}
