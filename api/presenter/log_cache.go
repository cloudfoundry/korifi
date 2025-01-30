package presenter

import (
	"strconv"

	"code.cloudfoundry.org/korifi/api/actions"
	"code.cloudfoundry.org/korifi/api/repositories"
	"code.cloudfoundry.org/korifi/tools"
)

type isLogCacheEnvelope interface {
	isLogCacheEnvelope()
}

type LogCacheReadResponse[E isLogCacheEnvelope] struct {
	Envelopes LogCacheReadResponseEnvelopes[E] `json:"envelopes"`
}

type LogCacheReadResponseEnvelopes[E isLogCacheEnvelope] struct {
	Batch []E `json:"batch"`
}

type Envelope struct {
	Timestamp int64             `json:"timestamp"`
	Tags      map[string]string `json:"tags,omitempty"`
}

type LogEnvelope struct {
	Envelope
	Log Log `json:"log"`
}

func (LogEnvelope) isLogCacheEnvelope() {}

type GaugeEnvelope struct {
	Envelope
	Gauge Gauge `json:"gauge"`
}

func (GaugeEnvelope) isLogCacheEnvelope() {}

type Gauge struct {
	Metrics map[string]GaugeValue `json:"metrics"`
}

type GaugeValue struct {
	Unit  string      `json:"unit"`
	Value GaugeNumber `json:"value"`
}

type GaugeNumber interface {
	MarshalJSON() ([]byte, error)
}

type GaugeFloat float64

func (f GaugeFloat) MarshalJSON() ([]byte, error) {
	return []byte(strconv.FormatFloat(float64(f), 'f', -1, 64)), nil
}

type GaugeInt int64

func (i GaugeInt) MarshalJSON() ([]byte, error) {
	return []byte(strconv.FormatInt(int64(i), 10)), nil
}

type Log struct {
	Payload []byte  `json:"payload"`
	Type    LogType `json:"type"`
}

type LogType int

const (
	LOG_OUT LogType = iota
)

func ForLogs(logRecords []repositories.LogRecord) LogCacheReadResponse[LogEnvelope] {
	batch := []LogEnvelope{}
	for _, logRecord := range logRecords {
		batch = append(batch, LogEnvelope{
			Envelope: Envelope{
				Timestamp: logRecord.Timestamp,
				Tags:      logRecord.Tags,
			},
			Log: Log{
				Payload: []byte(logRecord.Message),
				Type:    LOG_OUT,
			},
		})
	}

	return LogCacheReadResponse[LogEnvelope]{
		Envelopes: LogCacheReadResponseEnvelopes[LogEnvelope]{
			Batch: batch,
		},
	}
}

func ForStats(appRecord repositories.AppRecord, appPodStats []actions.PodStatsRecord) LogCacheReadResponse[GaugeEnvelope] {
	batch := []GaugeEnvelope{}

	for _, podStats := range appPodStats {
		batch = append(batch, GaugeEnvelope{
			Envelope: Envelope{
				Timestamp: tools.ZeroIfNil(podStats.Usage.Timestamp).Unix(),
				Tags: map[string]string{
					"app_id":       appRecord.GUID,
					"app_name":     appRecord.Name,
					"instance_id":  strconv.Itoa(podStats.Index),
					"process_type": podStats.Type,
					"source_id":    appRecord.GUID,
					"space_id":     appRecord.SpaceGUID,
				},
			},
			Gauge: Gauge{
				Metrics: map[string]GaugeValue{
					"cpu": {
						Unit:  "percentage",
						Value: GaugeFloat(tools.ZeroIfNil(podStats.Usage.CPU)),
					},
					"memory": {
						Unit:  "bytes",
						Value: GaugeInt(tools.ZeroIfNil(podStats.Usage.Mem)),
					},
					"disk": {
						Unit:  "bytes",
						Value: GaugeInt(tools.ZeroIfNil(podStats.Usage.Disk)),
					},
					"memory_quota": {
						Unit:  "bytes",
						Value: GaugeInt(tools.ZeroIfNil(podStats.MemQuota)),
					},
					"disk_quota": {
						Unit:  "bytes",
						Value: GaugeInt(tools.ZeroIfNil(podStats.DiskQuota)),
					},
				},
			},
		})
	}

	return LogCacheReadResponse[GaugeEnvelope]{
		Envelopes: LogCacheReadResponseEnvelopes[GaugeEnvelope]{
			Batch: batch,
		},
	}
}
