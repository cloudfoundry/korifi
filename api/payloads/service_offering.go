package payloads

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"code.cloudfoundry.org/korifi/api/payloads/params"
	"code.cloudfoundry.org/korifi/api/payloads/parse"
	"code.cloudfoundry.org/korifi/api/payloads/validation"
	"code.cloudfoundry.org/korifi/api/repositories"
	jellidation "github.com/jellydator/validation"
)

type ServiceOfferingGet struct {
	IncludeResourceRules []params.IncludeResourceRule
}

func (g ServiceOfferingGet) Validate() error {
	return jellidation.ValidateStruct(&g,
		jellidation.Field(&g.IncludeResourceRules, jellidation.Each(jellidation.By(func(value any) error {
			rule, ok := value.(params.IncludeResourceRule)
			if !ok {
				return fmt.Errorf("%T is not supported, IncludeResourceRule is expected", value)
			}

			relationshipsPath := strings.Join(rule.RelationshipPath, ".")
			if relationshipsPath != "service_broker" {
				return jellidation.NewError("invalid_fields_param", "must be fields[service_broker]")
			}

			return jellidation.Each(validation.OneOf(
				"name",
				"guid",
			)).Validate(rule.Fields)
		}))),
	)
}

func (g ServiceOfferingGet) SupportedKeys() []string {
	return []string{"fields[service_broker]"}
}

func (l *ServiceOfferingGet) DecodeFromURLValues(values url.Values) error {
	l.IncludeResourceRules = append(l.IncludeResourceRules, params.ParseFields(values)...)
	return nil
}

type ServiceOfferingUpdate struct {
	Metadata MetadataPatch `json:"metadata"`
}

func (g ServiceOfferingUpdate) Validate() error {
	return jellidation.ValidateStruct(&g,
		jellidation.Field(&g.Metadata),
	)
}

func (c *ServiceOfferingUpdate) ToMessage(serviceOfferingGUID string) repositories.UpdateServiceOfferingMessage {
	return repositories.UpdateServiceOfferingMessage{
		GUID: serviceOfferingGUID,
		MetadataPatch: repositories.MetadataPatch{
			Labels:      c.Metadata.Labels,
			Annotations: c.Metadata.Annotations,
		},
	}
}

type ServiceOfferingList struct {
	Names                string
	BrokerNames          string
	IncludeResourceRules []params.IncludeResourceRule
}

func (l ServiceOfferingList) Validate() error {
	return jellidation.ValidateStruct(&l,
		jellidation.Field(&l.IncludeResourceRules, jellidation.Each(jellidation.By(func(value any) error {
			rule, ok := value.(params.IncludeResourceRule)
			if !ok {
				return fmt.Errorf("%T is not supported, IncludeResourceRule is expected", value)
			}

			relationshipsPath := strings.Join(rule.RelationshipPath, ".")
			if relationshipsPath != "service_broker" {
				return jellidation.NewError("invalid_fields_param", "must be fields[service_broker]")
			}

			return jellidation.Each(validation.OneOf(
				"name",
				"guid",
			)).Validate(rule.Fields)
		}))),
	)
}

func (l *ServiceOfferingList) ToMessage() repositories.ListServiceOfferingMessage {
	return repositories.ListServiceOfferingMessage{
		Names:       parse.ArrayParam(l.Names),
		BrokerNames: parse.ArrayParam(l.BrokerNames),
	}
}

func (l *ServiceOfferingList) SupportedKeys() []string {
	return []string{"names", "service_broker_names", "fields[service_broker]", "page", "per_page"}
}

func (l *ServiceOfferingList) IgnoredKeys() []*regexp.Regexp {
	return nil
}

func (l *ServiceOfferingList) DecodeFromURLValues(values url.Values) error {
	l.Names = values.Get("names")
	l.BrokerNames = values.Get("service_broker_names")
	l.IncludeResourceRules = append(l.IncludeResourceRules, params.ParseFields(values)...)
	return nil
}

type ServiceOfferingDelete struct {
	Purge bool
}

func (d *ServiceOfferingDelete) SupportedKeys() []string {
	return []string{"purge"}
}

func (d *ServiceOfferingDelete) DecodeFromURLValues(values url.Values) error {
	var err error
	if d.Purge, err = getBool(values, "purge"); err != nil {
		return err
	}

	return nil
}

func (d *ServiceOfferingDelete) ToMessage(guid string) repositories.DeleteServiceOfferingMessage {
	return repositories.DeleteServiceOfferingMessage{
		GUID:  guid,
		Purge: d.Purge,
	}
}
