package payloads

import (
	"encoding/json"
	"net/url"
	"regexp"

	"code.cloudfoundry.org/korifi/api/payloads/parse"
	"code.cloudfoundry.org/korifi/api/payloads/validation"
	"code.cloudfoundry.org/korifi/api/repositories"
	jellidation "github.com/jellydator/validation"
)

type ServiceBrokerCreate struct {
	Name           string                      `json:"name"`
	URL            string                      `json:"url"`
	Relationships  *ServiceBrokerRelationships `json:"relationships"`
	Authentication ServiceBrokerAuthentication `json:"authentication"`
	Metadata       Metadata                    `json:"metadata"`
}

type ServiceBrokerRelationships struct {
	Space *Relationship `json:"space"`
}

type ServiceBrokerAuthentication struct {
	Type        string                   `json:"type"`
	Credentials ServiceBrokerCredentials `json:"credentials"`
}

type ServiceBrokerCredentials struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (c ServiceBrokerCreate) Validate() error {
	return jellidation.ValidateStruct(&c,
		jellidation.Field(&c.Name, jellidation.Required),
		jellidation.Field(&c.URL, jellidation.Required),
		jellidation.Field(&c.Authentication, jellidation.Required),
	)
}

func (p ServiceBrokerCreate) ToServiceBrokerCreateMessage() repositories.CreateServiceBrokerMessage {
	message := repositories.CreateServiceBrokerMessage{
		Name: p.Name,
		Credentials: map[string]string{
			"username": p.Authentication.Credentials.Username,
			"password": p.Authentication.Credentials.Password,
		},
		URL:         p.URL,
		Labels:      p.Metadata.Labels,
		Annotations: p.Metadata.Annotations,
	}

	return message
}

type ServiceBrokerPatch struct {
	Name        *string            `json:"name,omitempty"`
	URL         *string            `json:"url,omitempty"`
	Credentials *map[string]string `json:"credentials,omitempty"`
	Metadata    MetadataPatch      `json:"metadata"`
}

func (p ServiceBrokerPatch) Validate() error {
	return jellidation.ValidateStruct(&p,
		jellidation.Field(&p.Metadata),
	)
}

func (p ServiceBrokerPatch) ToServiceBrokerPatchMessage(brokerGUID string) repositories.PatchServiceBrokerMessage {
	return repositories.PatchServiceBrokerMessage{
		GUID:        brokerGUID,
		Name:        p.Name,
		Credentials: p.Credentials,
		URL:         p.URL,
		MetadataPatch: repositories.MetadataPatch{
			Labels:      p.Metadata.Labels,
			Annotations: p.Metadata.Annotations,
		},
	}
}

func (p *ServiceBrokerPatch) UnmarshalJSON(data []byte) error {
	type alias ServiceBrokerPatch

	var patch alias
	err := json.Unmarshal(data, &patch)
	if err != nil {
		return err
	}

	var patchMap map[string]any
	err = json.Unmarshal(data, &patchMap)
	if err != nil {
		return err
	}

	if v, ok := patchMap["credentials"]; ok && v == nil {
		patch.Credentials = &map[string]string{}
	}

	*p = ServiceBrokerPatch(patch)

	return nil
}

type ServiceBrokerList struct {
	Names         string
	GUIDs         string
	OrderBy       string
	LabelSelector string
}

func (l ServiceBrokerList) Validate() error {
	return jellidation.ValidateStruct(&l,
		jellidation.Field(&l.OrderBy, validation.OneOfOrderBy("created_at", "name", "updated_at")),
	)
}

func (l *ServiceBrokerList) ToMessage() repositories.ListServiceBrokerMessage {
	return repositories.ListServiceBrokerMessage{
		Names:         parse.ArrayParam(l.Names),
		GUIDs:         parse.ArrayParam(l.GUIDs),
		LabelSelector: l.LabelSelector,
	}
}

func (l *ServiceBrokerList) SupportedKeys() []string {
	return []string{"names", "guids", "order_by", "per_page", "page", "label_selector"}
}

func (l *ServiceBrokerList) IgnoredKeys() []*regexp.Regexp {
	return []*regexp.Regexp{regexp.MustCompile(`fields\[.+\]`)}
}

func (l *ServiceBrokerList) DecodeFromURLValues(values url.Values) error {
	l.Names = values.Get("names")
	l.GUIDs = values.Get("guids")
	l.OrderBy = values.Get("order_by")
	l.LabelSelector = values.Get("label_selector")
	return nil
}
