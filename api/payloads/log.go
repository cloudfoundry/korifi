package payloads

type LogRead struct {
	StartTime     *int64   `schema:"start_time"`
	EndTime       *int64   `schema:"end_time"`
	EnvelopeTypes []string `schema:"envelope_types" validate:"dive,eq=LOG|eq=COUNTER|eq=GAUGE|eq=TIMER|eq=EVENT"`
	Limit         *int64   `schema:"limit"`
	Descending    *bool    `schema:"descending"`
}

func (l *LogRead) SupportedKeys() []string {
	return []string{"start_time", "end_time", "envelope_types", "limit", "descending"}
}
