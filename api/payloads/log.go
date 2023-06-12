package payloads

import (
	"net/url"
	"strconv"
)

type LogRead struct {
	StartTime     int64
	EnvelopeTypes []string `validate:"dive,eq=LOG|eq=COUNTER|eq=GAUGE|eq=TIMER|eq=EVENT"`
	Limit         int64
	Descending    bool
}

func (l *LogRead) SupportedKeys() []string {
	return []string{"start_time", "end_time", "envelope_types", "limit", "descending"}
}

func (l *LogRead) DecodeFromURLValues(values url.Values) error {
	var err error
	if l.StartTime, err = getInt(values, "start_time"); err != nil {
		return err
	}
	l.EnvelopeTypes = values["envelope_types"]
	if l.Limit, err = getInt(values, "limit"); err != nil {
		return err
	}
	if l.Descending, err = getBool(values, "descending"); err != nil {
		return err
	}
	return nil
}

func getInt(values url.Values, key string) (int64, error) {
	if !values.Has(key) {
		return 0, nil
	}
	s := values.Get(key)
	if s == "" {
		return 0, nil
	}
	return strconv.ParseInt(s, 10, 64)
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
