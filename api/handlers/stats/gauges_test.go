package stats_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"time"

	"code.cloudfoundry.org/korifi/api/authorization"
	"code.cloudfoundry.org/korifi/api/handlers/stats"
	"code.cloudfoundry.org/korifi/api/handlers/stats/fake"
	"code.cloudfoundry.org/korifi/tools"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
)

var _ = Describe("LogCacheGaugesCollector", func() {
	var (
		logCacheURL          *url.URL
		logCacheReadHandler  *fake.HttpHandler
		logCacheReadResponse map[string]any

		gaugesCollector *stats.LogCacheGaugesCollector
		gauges          []stats.ProcessGauges
		gaugesErr       error
	)

	BeforeEach(func() {
		ctx = authorization.NewContext(ctx, &authorization.Info{
			RawAuthHeader: "bearer token",
		})

		logCacheReadResponse = map[string]any{
			"envelopes": map[string]any{
				"batch": []map[string]any{
					{
						"timestamp": 2000,
						"tags": map[string]string{
							"instance_id":  "3",
							"source_id":    "app-guid",
							"process_type": "web",
							"process_id":   "process-guid",
						},
						"gauge": map[string]any{
							"metrics": map[string]any{
								"cpu": map[string]any{
									"unit":  "percentage",
									"value": 1.23,
								},

								"disk": map[string]any{
									"unit":  "bytes",
									"value": 6665,
								},
								"disk_quota": map[string]any{
									"unit":  "bytes",
									"value": 6666,
								},

								"memory": map[string]any{
									"unit":  "bytes",
									"value": 7776,
								},
								"memory_quota": map[string]any{
									"unit":  "bytes",
									"value": 7777,
								},
							},
						},
					},
				},
			},
		}

		logCacheReadHandler = new(fake.HttpHandler)
		logCacheReadHandler.ServeHTTPStub = func(w http.ResponseWriter, _ *http.Request) {
			responseBytes, err := json.Marshal(logCacheReadResponse)
			Expect(err).NotTo(HaveOccurred())

			_, err = w.Write(responseBytes)
			Expect(err).NotTo(HaveOccurred())

			w.WriteHeader(http.StatusOK)
		}

		mux := http.NewServeMux()
		mux.HandleFunc("/api/v1/info", func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"version": "2.11.4"}`))
			w.WriteHeader(http.StatusOK)
		})
		mux.Handle("/api/v1/read/{sourceId}", logCacheReadHandler)
		logCacheServer := httptest.NewServer(mux)
		DeferCleanup(func() {
			logCacheServer.Close()
		})

		var err error
		logCacheURL, err = url.Parse(logCacheServer.URL)
		Expect(err).NotTo(HaveOccurred())

		gaugesCollector = stats.NewGaugesCollector(logCacheURL.String(), http.DefaultClient)
	})

	JustBeforeEach(func() {
		gauges, gaugesErr = gaugesCollector.CollectProcessGauges(ctx, "app-guid", "process-guid")
	})

	It("impersonates the user from the request context", func() {
		Expect(logCacheReadHandler.ServeHTTPCallCount()).To(Equal(1))
		_, req := logCacheReadHandler.ServeHTTPArgsForCall(0)

		Expect(req.Header).To(MatchKeys(IgnoreExtras, Keys{
			"Authorization": ConsistOf("bearer token"),
		}))
	})

	It("sends the correct request parameters", func() {
		Expect(logCacheReadHandler.ServeHTTPCallCount()).To(Equal(1))
		_, req := logCacheReadHandler.ServeHTTPArgsForCall(0)
		Expect(req.ParseForm()).To(Succeed())

		Expect(req.Form).To(MatchKeys(IgnoreExtras, Keys{
			"envelope_types": ConsistOf(Equal("GAUGE")),
			"descending":     ConsistOf(Equal("true")),
			"limit":          ConsistOf(Equal("1000")),
		}))

		startTime, err := strconv.ParseInt(req.FormValue("start_time"), 10, 64)
		Expect(err).NotTo(HaveOccurred())
		Expect(startTime).To(BeNumerically("~", time.Now().Add(-2*time.Minute).UTC().UnixNano(), time.Second.Nanoseconds()))

		endTime, err := strconv.ParseInt(req.FormValue("end_time"), 10, 64)
		Expect(err).NotTo(HaveOccurred())
		Expect(endTime).To(BeNumerically("~", time.Now().UTC().UnixNano(), time.Second.Nanoseconds()))
	})

	When("logcache returns an error", func() {
		BeforeEach(func() {
			logCacheReadHandler.ServeHTTPStub = func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusTeapot)
			}
		})

		It("returns an error", func() {
			Expect(gaugesErr).To(MatchError(ContainSubstring(strconv.Itoa(http.StatusTeapot))))
		})
	})

	It("returns the proces stats", func() {
		Expect(gaugesErr).NotTo(HaveOccurred())
		Expect(gauges).To(ConsistOf(stats.ProcessGauges{
			Index:     3,
			CPU:       tools.PtrTo(1.23),
			Mem:       tools.PtrTo[int64](7776),
			Disk:      tools.PtrTo[int64](6665),
			MemQuota:  tools.PtrTo[int64](7777),
			DiskQuota: tools.PtrTo[int64](6666),
		}))
	})

	When("logcache returns historical data", func() {
		BeforeEach(func() {
			logCacheReadResponse = map[string]any{
				"envelopes": map[string]any{
					"batch": []map[string]any{
						{
							"timestamp": 3000,
							"tags": map[string]string{
								"instance_id":  "3",
								"source_id":    "app-guid",
								"process_type": "web",
								"process_id":   "process-guid",
							},
							"gauge": map[string]any{
								"metrics": map[string]any{
									"cpu": map[string]any{
										"unit":  "percentage",
										"value": 4.56,
									},
								},
							},
						},
						{
							"timestamp": 2000,
							"tags": map[string]string{
								"instance_id":  "3",
								"source_id":    "app-guid",
								"process_type": "web",
								"process_id":   "process-guid",
							},
							"gauge": map[string]any{
								"metrics": map[string]any{
									"cpu": map[string]any{
										"unit":  "percentage",
										"value": 1.23,
									},
								},
							},
						},
					},
				},
			}
		})

		It("returns latest metric", func() {
			Expect(gaugesErr).NotTo(HaveOccurred())

			Expect(gauges).To(ConsistOf(stats.ProcessGauges{
				Index: 3,
				CPU:   tools.PtrTo(4.56),
			}))
		})
	})

	When("metrics are split in multiple batches", func() {
		BeforeEach(func() {
			logCacheReadResponse = map[string]any{
				"envelopes": map[string]any{
					"batch": []map[string]any{
						{
							"timestamp": 2000,
							"tags": map[string]string{
								"instance_id":  "0",
								"source_id":    "app-guid",
								"process_type": "web",
								"process_id":   "process-guid",
							},
							"gauge": map[string]any{
								"metrics": map[string]any{
									"cpu": map[string]any{
										"unit":  "percentage",
										"value": 1.23,
									},
								},
							},
						},
						{
							"timestamp": 2000,
							"tags": map[string]string{
								"instance_id":  "0",
								"source_id":    "app-guid",
								"process_type": "web",
								"process_id":   "process-guid",
							},
							"gauge": map[string]any{
								"metrics": map[string]any{
									"disk": map[string]any{
										"unit":  "bytes",
										"value": 6665,
									},
								},
							},
						},
						{
							"timestamp": 2000,
							"tags": map[string]string{
								"instance_id":  "1",
								"source_id":    "app-guid",
								"process_type": "web",
								"process_id":   "process-guid",
							},
							"gauge": map[string]any{
								"metrics": map[string]any{
									"disk": map[string]any{
										"unit":  "bytes",
										"value": 1111,
									},
								},
							},
						},
					},
				},
			}
		})

		It("aggregates them", func() {
			Expect(gaugesErr).NotTo(HaveOccurred())

			Expect(gauges).To(ConsistOf(stats.ProcessGauges{
				Index: 0,
				CPU:   tools.PtrTo(1.23),
				Disk:  tools.PtrTo[int64](6665),
			}, stats.ProcessGauges{
				Index: 1,
				Disk:  tools.PtrTo[int64](1111),
			}))
		})
	})

	When("there are metrics for multiple process types", func() {
		BeforeEach(func() {
			logCacheReadResponse = map[string]any{
				"envelopes": map[string]any{
					"batch": []map[string]any{
						{
							"timestamp": 2000,
							"tags": map[string]string{
								"process_id": "process-guid",
							},
							"gauge": map[string]any{
								"metrics": map[string]any{
									"cpu": map[string]any{
										"unit":  "percentage",
										"value": 1.23,
									},
								},
							},
						},
						{
							"timestamp": 2000,
							"tags": map[string]string{
								"process_id": "another-process-guid",
							},
							"gauge": map[string]any{
								"metrics": map[string]any{
									"disk": map[string]any{
										"unit":  "bytes",
										"value": 1111,
									},
								},
							},
						},
					},
				},
			}
		})

		It("returns stats for the desired process guid only", func() {
			Expect(gaugesErr).NotTo(HaveOccurred())

			Expect(gauges).To(ConsistOf(stats.ProcessGauges{
				Index: 0,
				CPU:   tools.PtrTo(1.23),
			}))
		})
	})
})
