package stats

import (
	"context"
	"maps"
	"net/http"
	"slices"
	"strconv"
	"time"

	client "code.cloudfoundry.org/go-log-cache/v3"
	"code.cloudfoundry.org/go-log-cache/v3/rpc/logcache_v1"
	"code.cloudfoundry.org/go-loggregator/v10/rpc/loggregator_v2"
	"code.cloudfoundry.org/korifi/api/authorization"
	"code.cloudfoundry.org/korifi/tools"
	"github.com/BooleanCat/go-functional/v2/it/itx"
	"golang.org/x/exp/constraints"
)

//go:generate go run github.com/maxbrunsfeld/counterfeiter/v6 -generate

//counterfeiter:generate -o fake -fake-name HttpHandler net/http.Handler

type ProcessGauges struct {
	Index     int
	CPU       *float64
	Mem       *int64
	Disk      *int64
	MemQuota  *int64
	DiskQuota *int64
}

type LogCacheGaugesCollector struct {
	logCacheClient *client.Client
}

func NewGaugesCollector(logCacheURL string, httpClient *http.Client) *LogCacheGaugesCollector {
	return &LogCacheGaugesCollector{
		logCacheClient: client.NewClient(logCacheURL, client.WithHTTPClient(&impersonatingHttpClient{
			httpClient: httpClient,
		})),
	}
}

type impersonatingHttpClient struct {
	httpClient *http.Client
}

func (c *impersonatingHttpClient) Do(req *http.Request) (*http.Response, error) {
	authInfo, _ := authorization.InfoFromContext(req.Context())
	req.Header.Set("Authorization", authInfo.RawAuthHeader)

	return c.httpClient.Do(req)
}

func (c *LogCacheGaugesCollector) CollectProcessGauges(ctx context.Context, appGUID, processGUID string) ([]ProcessGauges, error) {
	envelopes, err := c.logCacheClient.Read(ctx, appGUID, time.Now().Add(-2*time.Minute),
		client.WithEndTime(time.Now()),
		client.WithDescending(),
		client.WithLimit(1000),
		client.WithEnvelopeTypes(logcache_v1.EnvelopeType_GAUGE),
	)
	if err != nil {
		return nil, err
	}

	envelopes = itx.FromSlice(envelopes).Filter(func(e *loggregator_v2.Envelope) bool {
		return e.Tags["process_id"] == processGUID
	}).Collect()

	statsMap := map[string]ProcessGauges{}
	for _, e := range envelopes {
		tools.InsertOrUpdate(statsMap, e.Tags["instance_id"], func(stats *ProcessGauges) {
			stats.Index, _ = strconv.Atoi(e.Tags["instance_id"])

			metrics := e.GetGauge().GetMetrics()
			stats.CPU = tools.IfNil(stats.CPU, gaugeValueOrNil[float64](metrics["cpu"]))
			stats.Mem = tools.IfNil(stats.Mem, gaugeValueOrNil[int64](metrics["memory"]))
			stats.Disk = tools.IfNil(stats.Disk, gaugeValueOrNil[int64](metrics["disk"]))
			stats.MemQuota = tools.IfNil(stats.MemQuota, gaugeValueOrNil[int64](metrics["memory_quota"]))
			stats.DiskQuota = tools.IfNil(stats.DiskQuota, gaugeValueOrNil[int64](metrics["disk_quota"]))
		})
	}

	return slices.Collect(maps.Values(statsMap)), nil
}

type gaugeNumber interface {
	constraints.Integer | constraints.Float
}

func gaugeValueOrNil[T gaugeNumber](v *loggregator_v2.GaugeValue) *T {
	if v == nil {
		return nil
	}

	return tools.PtrTo(T(v.GetValue()))
}
