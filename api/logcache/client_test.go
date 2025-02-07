package logcache_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"

	"code.cloudfoundry.org/korifi/api/logcache"
	"code.cloudfoundry.org/korifi/api/logcache/fake"
	"code.cloudfoundry.org/korifi/tools"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
)

var _ = Describe("Logcache Client", func() {
	var (
		client           *logcache.LogCacheClient
		logCacheHandler  *fake.HttpHandler
		logCacheURL      *url.URL
		logCacheResponse map[string]any
		statsResponse    logcache.LogCacheGaugeResponse
		statsErr         error
	)

	BeforeEach(func() {
		logCacheResponse = map[string]any{
			"envelopes": map[string]any{
				"batch": []map[string]any{
					{
						"timestamp": "2000",
						"tags": map[string]string{
							"instance_id":  "0",
							"source_id":    "app-guid",
							"process_type": "web",
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

		logCacheHandler = new(fake.HttpHandler)
		logCacheHandler.ServeHTTPStub = func(w http.ResponseWriter, _ *http.Request) {
			responseBytes, err := json.Marshal(logCacheResponse)
			Expect(err).NotTo(HaveOccurred())

			_, err = w.Write(responseBytes)
			Expect(err).NotTo(HaveOccurred())
		}

		logCacheServer := httptest.NewServer(logCacheHandler)
		DeferCleanup(func() {
			logCacheServer.Close()
		})

		var err error
		logCacheURL, err = url.Parse(logCacheServer.URL)
		Expect(err).NotTo(HaveOccurred())

		client = logcache.NewLogCacheClient(*logCacheURL)
	})

	JustBeforeEach(func() {
		statsResponse, statsErr = client.GetStats(ctx, "app-guid")
	})

	It("fetches stats for the process from the log cache", func() {
		Expect(statsErr).NotTo(HaveOccurred())

		Expect(logCacheHandler.ServeHTTPCallCount()).To(Equal(1))
		_, req := logCacheHandler.ServeHTTPArgsForCall(0)
		Expect(req.Method).To(Equal(http.MethodGet))
		Expect(req.URL.String()).To(Equal(logCacheURL.RawPath + "/api/v1/read/app-guid?envelope_types=GAUGE"))
	})

	It("returns the stats", func() {
		Expect(statsErr).NotTo(HaveOccurred())

		Expect(statsResponse).To(Equal(logcache.LogCacheGaugeResponse{
			Envelopes: logcache.GaugeEnvelopes{
				Batch: []logcache.GaugeEnvelope{
					{
						Timestamp: "2000",
						Tags: map[string]string{
							"instance_id":  "0",
							"source_id":    "app-guid",
							"process_type": "web",
						},
						Gauge: logcache.Gauge{
							Metrics: logcache.GaugeMetrics{
								CPU: tools.PtrTo(logcache.GaugeFloatValue{
									Unit:  "percentage",
									Value: 1.23,
								}),
								Memory: tools.PtrTo(logcache.GaugeIntValue{
									Unit:  "bytes",
									Value: 7776,
								}),
								MemoryQuota: tools.PtrTo(logcache.GaugeIntValue{
									Unit:  "bytes",
									Value: 7777,
								}),
								Disk: tools.PtrTo(logcache.GaugeIntValue{
									Unit:  "bytes",
									Value: 6665,
								}),
								DiskQuota: tools.PtrTo(logcache.GaugeIntValue{
									Unit:  "bytes",
									Value: 6666,
								}),
							},
						},
					},
				},
			},
		}))
	})

	When("logcache returns an error", func() {
		BeforeEach(func() {
			logCacheHandler.ServeHTTPStub = func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusTeapot)
			}
		})

		It("returns an error", func() {
			Expect(statsErr).To(MatchError(ContainSubstring(strconv.Itoa(http.StatusTeapot))))
		})
	})

	When("logcache returns invalid response", func() {
		BeforeEach(func() {
			logCacheHandler.ServeHTTPStub = func(w http.ResponseWriter, _ *http.Request) {
				w.Write([]byte("not-json"))
			}
		})

		It("returns an error", func() {
			Expect(statsErr).To(MatchError(ContainSubstring("invalid")))
		})
	})

	When("logcache returns historical data", func() {
		BeforeEach(func() {
			logCacheResponse = map[string]any{
				"envelopes": map[string]any{
					"batch": []map[string]any{
						{
							"timestamp": "2000",
							"tags": map[string]string{
								"instance_id":  "0",
								"source_id":    "app-guid",
								"process_type": "web",
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
							"timestamp": "3000",
							"tags": map[string]string{
								"instance_id":  "0",
								"source_id":    "app-guid",
								"process_type": "web",
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
					},
				},
			}
		})

		FIt("returns latest metric", func() {
			Expect(statsErr).NotTo(HaveOccurred())

			Expect(statsResponse).To(Equal(logcache.LogCacheGaugeResponse{
				Envelopes: logcache.GaugeEnvelopes{
					Batch: []logcache.GaugeEnvelope{
						{
							Timestamp: "3000",
							Tags: map[string]string{
								"instance_id":  "0",
								"source_id":    "app-guid",
								"process_type": "web",
							},
							Gauge: logcache.Gauge{
								Metrics: logcache.GaugeMetrics{
									CPU: tools.PtrTo(logcache.GaugeFloatValue{
										Unit:  "percentage",
										Value: 4.56,
									}),
								},
							},
						},
					},
				},
			}))
		})
	})

	When("metrics are split in multiple batches", func() {
		BeforeEach(func() {
			logCacheResponse = map[string]any{
				"envelopes": map[string]any{
					"batch": []map[string]any{
						{
							"timestamp": "2000",
							"tags": map[string]string{
								"instance_id":  "0",
								"source_id":    "app-guid",
								"process_type": "web",
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
							"timestamp": "2000",
							"tags": map[string]string{
								"instance_id":  "0",
								"source_id":    "app-guid",
								"process_type": "web",
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
							"timestamp": "2000",
							"tags": map[string]string{
								"instance_id":  "1",
								"source_id":    "app-guid",
								"process_type": "web",
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
						{
							"timestamp": "2000",
							"tags": map[string]string{
								"instance_id":  "0",
								"source_id":    "app-guid",
								"process_type": "worker",
							},
							"gauge": map[string]any{
								"metrics": map[string]any{
									"disk": map[string]any{
										"unit":  "bytes",
										"value": 2222,
									},
								},
							},
						},
					},
				},
			}
		})

		It("normalizes the envelopes", func() {
			Expect(statsErr).NotTo(HaveOccurred())
			Expect(statsResponse).To(MatchAllFields(Fields{
				"Envelopes": MatchAllFields(Fields{
					"Batch": ConsistOf(
						MatchFields(IgnoreExtras, Fields{
							"Timestamp": Equal("2000"),
							"Tags": SatisfyAll(
								HaveKeyWithValue("instance_id", "0"),
								HaveKeyWithValue("process_type", "web"),
							),
							"Gauge": MatchAllFields(Fields{
								"Metrics": MatchFields(IgnoreExtras, Fields{
									"CPU": PointTo(Equal(logcache.GaugeFloatValue{
										Unit:  "percentage",
										Value: 1.23,
									})),
									"Memory":      BeNil(),
									"MemoryQuota": BeNil(),
									"Disk": PointTo(Equal(logcache.GaugeIntValue{
										Unit:  "bytes",
										Value: 6665,
									})),
									"DiskQuota": BeNil(),
								}),
							}),
						}),
						MatchFields(IgnoreExtras, Fields{
							"Timestamp": Equal("2000"),
							"Tags": SatisfyAll(
								HaveKeyWithValue("instance_id", "1"),
								HaveKeyWithValue("process_type", "web"),
							),
							"Gauge": MatchAllFields(Fields{
								"Metrics": MatchAllFields(Fields{
									"CPU":         BeNil(),
									"Memory":      BeNil(),
									"MemoryQuota": BeNil(),
									"Disk": PointTo(Equal(logcache.GaugeIntValue{
										Unit:  "bytes",
										Value: 1111,
									})),
									"DiskQuota": BeNil(),
								}),
							}),
						}),
						MatchFields(IgnoreExtras, Fields{
							"Timestamp": Equal("2000"),
							"Tags": SatisfyAll(
								HaveKeyWithValue("instance_id", "0"),
								HaveKeyWithValue("process_type", "worker"),
							),
							"Gauge": MatchAllFields(Fields{
								"Metrics": MatchAllFields(Fields{
									"CPU":         BeNil(),
									"Memory":      BeNil(),
									"MemoryQuota": BeNil(),
									"Disk": PointTo(Equal(logcache.GaugeIntValue{
										Unit:  "bytes",
										Value: 2222,
									})),
									"DiskQuota": BeNil(),
								}),
							}),
						}),
					),
				}),
			}))
		})
	})
})
