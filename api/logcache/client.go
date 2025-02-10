package logcache

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"code.cloudfoundry.org/go-loggregator/v8/rpc/loggregator_v2"
	"code.cloudfoundry.org/korifi/tools"
)

//go:generate go run github.com/maxbrunsfeld/counterfeiter/v6 -generate

//counterfeiter:generate -o fake -fake-name HttpHandler net/http.Handler

type LogCacheGaugeResponse struct {
	Envelopes EnvelopeBatch `json:"envelopes"`
}

type EnvelopeBatch struct {
	Batch []loggregator_v2.Envelope `json:"batch"`
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

func envelopeKey(envelope *loggregator_v2.Envelope) string {
	return fmt.Sprintf("%s#%s", envelope.Tags["process_type"], envelope.Tags["instance_id"])
}

func putIfAbsent(m map[string][]*loggregator_v2.Envelope, key string, envelope *loggregator_v2.Envelope) *loggregator_v2.Envelope {
	envelopes, found := m[key]
	if !found {
		m[key] = []*loggregator_v2.Envelope{envelope}
		return envelope
	}

	var foundEnvelope *loggregator_v2.Envelope
	for _, e := range envelopes {
		if envelopeKey(e) == envelopeKey(envelope) {
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
	envelopesByProcessInstance := map[string][]*loggregator_v2.Envelope{}

	for _, rawEnvelope := range rawResponse.Envelopes.Batch {
		processInstanceEnvelope := putIfAbsent(envelopesByProcessInstance, envelopeKey(&rawEnvelope), tools.PtrTo(rawEnvelope))
		processInstanceEnvelope.GetGauge().Metrics["cpu"] = ifNil(processInstanceEnvelope.GetGauge().Metrics["cpu"], rawEnvelope.GetGauge().Metrics["cpu"])
		processInstanceEnvelope.GetGauge().Metrics["memory"] = ifNil(processInstanceEnvelope.GetGauge().Metrics["cpu"], rawEnvelope.GetGauge().Metrics["memory"])
		processInstanceEnvelope.GetGauge().Metrics["disk"] = ifNil(processInstanceEnvelope.GetGauge().Metrics["cpu"], rawEnvelope.GetGauge().Metrics["disk"])
		processInstanceEnvelope.GetGauge().Metrics["memory_quota"] = ifNil(processInstanceEnvelope.GetGauge().Metrics["cpu"], rawEnvelope.GetGauge().Metrics["memory"])
		processInstanceEnvelope.GetGauge().Metrics["disk_quota"] = ifNil(processInstanceEnvelope.GetGauge().Metrics["cpu"], rawEnvelope.GetGauge().Metrics["disk_quota"])
	}

	result := LogCacheGaugeResponse{
		Envelopes: EnvelopeBatch{
			Batch: []loggregator_v2.Envelope{},
		},
	}

	for _, envelope := range envelopesByProcessInstance {
		result.Envelopes.Batch = append(result.Envelopes.Batch, *envelope[0])
	}

	return result
}
