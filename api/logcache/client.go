package logcache

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"code.cloudfoundry.org/korifi/tools"
)

//go:generate go run github.com/maxbrunsfeld/counterfeiter/v6 -generate

//counterfeiter:generate -o fake -fake-name HttpHandler net/http.Handler

type LogCacheGaugeResponse struct {
	Envelopes GaugeEnvelopes `json:"envelopes"`
}

type GaugeEnvelopes struct {
	Batch []GaugeEnvelope `json:"batch"`
}

type GaugeEnvelope struct {
	Timestamp string            `json:"timestamp"`
	Tags      map[string]string `json:"tags,omitempty"`
	Gauge     Gauge             `json:"gauge"`
}

type Gauge struct {
	Metrics GaugeMetrics `json:"metrics"`
}

type GaugeMetrics struct {
	CPU         *GaugeFloatValue `json:"cpu"`
	Memory      *GaugeIntValue   `json:"memory"`
	Disk        *GaugeIntValue   `json:"disk"`
	MemoryQuota *GaugeIntValue   `json:"memory_quota"`
	DiskQuota   *GaugeIntValue   `json:"disk_quota"`
}

type GaugeIntValue struct {
	Unit  string `json:"unit"`
	Value int    `json:"value"`
}

type GaugeFloatValue struct {
	Unit  string  `json:"unit"`
	Value float64 `json:"value"`
}

type Client interface {
	GetStats(ctx, appGUID string) (LogCacheGaugeResponse, error)
}

type LogCacheClient struct {
	logCacheURL url.URL
}

func NewLogCacheClient(logCacheURL url.URL) *LogCacheClient {
	return &LogCacheClient{
		logCacheURL: logCacheURL,
	}
}

func (c *LogCacheClient) GetStats(ctx context.Context, appGUID string) (LogCacheGaugeResponse, error) {
	requestURL, err := url.Parse(c.logCacheURL.String() + "/api/v1/read/" + appGUID + "?envelope_types=GAUGE")
	if err != nil {
		return LogCacheGaugeResponse{}, fmt.Errorf("failed to build logcache request url: %w", err)
	}

	response, err := http.Get(requestURL.String())
	if err != nil {
		return LogCacheGaugeResponse{}, fmt.Errorf("failed to execute logcache request %q: %w", requestURL, err)
	}

	if response.StatusCode != http.StatusOK {
		return LogCacheGaugeResponse{}, fmt.Errorf("logcache request failed with status code %d", response.StatusCode)
	}

	responseBytes, err := io.ReadAll(response.Body)
	if err != nil {
		return LogCacheGaugeResponse{}, fmt.Errorf("failed to read logcache response body: %w", err)
	}
	defer response.Body.Close()

	stats := LogCacheGaugeResponse{}
	err = json.Unmarshal(responseBytes, &stats)
	if err != nil {
		return LogCacheGaugeResponse{}, fmt.Errorf("logcache response body is invalid: %w", err)
	}

	stats = nozmalize(stats)
	return stats, nil
}

func envelopeKey(envelope GaugeEnvelope) string {
	return fmt.Sprintf("%s#%s", envelope.Tags["process_type"], envelope.Tags["instance_id"])
}

func putIfAbsent(m map[string][]*GaugeEnvelope, key string, envelope *GaugeEnvelope) *GaugeEnvelope {
	envelopes, found := m[key]
	if !found {
		m[key] = []*GaugeEnvelope{envelope}
		return envelope
	}

	var foundEnvelope *GaugeEnvelope
	for _, e := range envelopes {
		if envelopeKey(*e) == envelopeKey(*envelope) {
			foundEnvelope = e
			break
		}
	}
	return foundEnvelope
}

func ifNil[T any, PT *T](v PT, ret PT) PT {
	if v == nil {
		return ret
	}

	return v
}

func nozmalize(rawResponse LogCacheGaugeResponse) LogCacheGaugeResponse {
	envelopesByProcessInstance := map[string][]*GaugeEnvelope{}

	for _, rawEnvelope := range rawResponse.Envelopes.Batch {
		processInstanceEnvelope := putIfAbsent(envelopesByProcessInstance, envelopeKey(rawEnvelope), tools.PtrTo(rawEnvelope))
		processInstanceEnvelope.Gauge.Metrics.CPU = ifNil(processInstanceEnvelope.Gauge.Metrics.CPU, rawEnvelope.Gauge.Metrics.CPU)
		processInstanceEnvelope.Gauge.Metrics.Memory = ifNil(processInstanceEnvelope.Gauge.Metrics.Memory, rawEnvelope.Gauge.Metrics.Memory)
		processInstanceEnvelope.Gauge.Metrics.MemoryQuota = ifNil(processInstanceEnvelope.Gauge.Metrics.MemoryQuota, rawEnvelope.Gauge.Metrics.MemoryQuota)
		processInstanceEnvelope.Gauge.Metrics.Disk = ifNil(processInstanceEnvelope.Gauge.Metrics.Disk, rawEnvelope.Gauge.Metrics.Disk)
		processInstanceEnvelope.Gauge.Metrics.DiskQuota = ifNil(processInstanceEnvelope.Gauge.Metrics.DiskQuota, rawEnvelope.Gauge.Metrics.DiskQuota)
	}

	result := LogCacheGaugeResponse{
		Envelopes: GaugeEnvelopes{
			Batch: []GaugeEnvelope{},
		},
	}

	for _, envelope := range envelopesByProcessInstance {
		result.Envelopes.Batch = append(result.Envelopes.Batch, *envelope[0])
	}

	return result
}
