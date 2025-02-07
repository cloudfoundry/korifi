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

	stats = normalizeStats(stats)

	return stats, nil
}

func envelopeKey(envelope GaugeEnvelope) string {
	return fmt.Sprintf("%s#%s#%s", envelope.Tags["process_type"], envelope.Tags["instance_id"], envelope.Timestamp)
}

func putIfAbsent(m map[string]*GaugeEnvelope, key string, envelope *GaugeEnvelope) *GaugeEnvelope {
	e, ok := m[key]
	if ok {
		return e
	}

	m[key] = envelope
	return envelope
}

func ifNil[T any, PT *T](v PT, ret PT) PT {
	if v == nil {
		return ret
	}

	return v
}

func normalizeStats(rawResponse LogCacheGaugeResponse) LogCacheGaugeResponse {
	normalizedEnvelopes := map[string]*GaugeEnvelope{}

	for _, e := range rawResponse.Envelopes.Batch {
		envelope := putIfAbsent(normalizedEnvelopes, envelopeKey(e), tools.PtrTo(e))
		envelope.Gauge.Metrics.CPU = ifNil(envelope.Gauge.Metrics.CPU, e.Gauge.Metrics.CPU)
		envelope.Gauge.Metrics.Memory = ifNil(envelope.Gauge.Metrics.Memory, e.Gauge.Metrics.Memory)
		envelope.Gauge.Metrics.MemoryQuota = ifNil(envelope.Gauge.Metrics.MemoryQuota, e.Gauge.Metrics.MemoryQuota)
		envelope.Gauge.Metrics.Disk = ifNil(envelope.Gauge.Metrics.Disk, e.Gauge.Metrics.Disk)
		envelope.Gauge.Metrics.DiskQuota = ifNil(envelope.Gauge.Metrics.DiskQuota, e.Gauge.Metrics.DiskQuota)
	}

	result := LogCacheGaugeResponse{
		Envelopes: GaugeEnvelopes{
			Batch: []GaugeEnvelope{},
		},
	}

	for _, envelope := range normalizedEnvelopes {
		result.Envelopes.Batch = append(result.Envelopes.Batch, *envelope)
	}

	return result
}
