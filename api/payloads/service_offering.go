package payloads

import (
	"fmt"
	"net/url"
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

func (g *ServiceOfferingGet) DecodeFromURLValues(values url.Values) error {
	g.IncludeResourceRules = append(g.IncludeResourceRules, params.ParseFields(values)...)
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

func (g *ServiceOfferingUpdate) ToMessage(serviceOfferingGUID string) repositories.UpdateServiceOfferingMessage {
	return repositories.UpdateServiceOfferingMessage{
		GUID: serviceOfferingGUID,
		MetadataPatch: repositories.MetadataPatch{
			Labels:      g.Metadata.Labels,
			Annotations: g.Metadata.Annotations,
		},
	}
}

type ServiceOfferingList struct {
	Names                string
	BrokerNames          string
	IncludeResourceRules []params.IncludeResourceRule
	OrderBy              string
	Pagination           Pagination
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
		jellidation.Field(&l.OrderBy, validation.OneOfOrderBy("created_at", "updated_at", "name")),
		jellidation.Field(&l.Pagination),
	)
}

func (l *ServiceOfferingList) ToMessage() repositories.ListServiceOfferingMessage {
	return repositories.ListServiceOfferingMessage{
		Names:       parse.ArrayParam(l.Names),
		BrokerNames: parse.ArrayParam(l.BrokerNames),
		OrderBy:     l.OrderBy,
		Pagination:  l.Pagination.ToMessage(DefaultPageSize),
	}
}

func (l *ServiceOfferingList) SupportedKeys() []string {
	return []string{"names", "service_broker_names", "fields[service_broker]", "order_by", "page", "per_page"}
}

func (l *ServiceOfferingList) DecodeFromURLValues(values url.Values) error {
	l.Names = values.Get("names")
	l.BrokerNames = values.Get("service_broker_names")
	l.IncludeResourceRules = append(l.IncludeResourceRules, params.ParseFields(values)...)
	l.OrderBy = values.Get("order_by")
	return l.Pagination.DecodeFromURLValues(values)
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
