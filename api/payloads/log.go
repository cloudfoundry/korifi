package payloads

import (
	"net/url"
	"strconv"

	"code.cloudfoundry.org/korifi/api/payloads/validation"
	jellidation "github.com/jellydator/validation"
)

type LogRead struct {
	StartTime     *int64
	EnvelopeTypes []string
	Limit         *int64
	Descending    bool
}

func (l LogRead) Validate() error {
	return jellidation.ValidateStruct(&l,
		jellidation.Field(&l.EnvelopeTypes,
			jellidation.Each(validation.OneOf("LOG")),
		),
	)
}

func (l *LogRead) SupportedKeys() []string {
	return []string{"start_time", "end_time", "envelope_types", "limit", "descending"}
}

func (l *LogRead) DecodeFromURLValues(values url.Values) error {
	var err error
	if l.StartTime, err = getIntPtr(values, "start_time"); err != nil {
		return err
	}
	l.EnvelopeTypes = values["envelope_types"]
	if l.Limit, err = getIntPtr(values, "limit"); err != nil {
		return err
	}
	if l.Descending, err = getBool(values, "descending"); err != nil {
		return err
	}
	return nil
}

func getIntPtr(values url.Values, key string) (*int64, error) {
	if !values.Has(key) {
		return nil, nil
	}

	result, err := strconv.ParseInt(values.Get(key), 10, 64)
	return &result, err
}

func getBool(values url.Values, key string) (bool, error) {
	if !values.Has(key) {
		return false, nil
	}
	s := values.Get(key)
	if s == "" {
		return false, nil
	}
	return strconv.ParseBool(s)
}
