package handlers_test

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"time"

	"code.cloudfoundry.org/korifi/api/apierrors"
	. "code.cloudfoundry.org/korifi/api/handlers"
	"code.cloudfoundry.org/korifi/api/handlers/fake"
	"code.cloudfoundry.org/korifi/api/repositories"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("LogCacheHandler", func() {
	var (
		appRepo       *fake.CFAppRepository
		buildRepo     *fake.CFBuildRepository
		appLogsReader *fake.AppLogsReader
		req           *http.Request
	)

	BeforeEach(func() {
		appRepo = new(fake.CFAppRepository)
		buildRepo = new(fake.CFBuildRepository)
		appLogsReader = new(fake.AppLogsReader)
		handler := NewLogCacheHandler(
			appRepo,
			buildRepo,
			appLogsReader,
		)
		handler.RegisterRoutes(router)
	})

	JustBeforeEach(func() {
		router.ServeHTTP(rr, req)
	})

	Describe("the GET /api/v1/info endpoint", func() {
		BeforeEach(func() {
			var err error
			req, err = http.NewRequest("GET", "/api/v1/info", nil)
			Expect(err).NotTo(HaveOccurred())
		})

		It("returns status 200 OK", func() {
			Expect(rr.Code).To(Equal(http.StatusOK))
		})

		It("returns Content-Type as JSON in header", func() {
			contentTypeHeader := rr.Header().Get("Content-Type")
			Expect(contentTypeHeader).To(Equal(jsonHeader))
		})

		It("matches the expected response body format", func() {
			expectedBody := `{"version":"2.11.4+cf-k8s","vm_uptime":"0"}`
			Expect(rr.Body).To(MatchJSON(expectedBody))
		})
	})

	Describe("the GET /api/v1/read/<app-guid> endpoint", func() {
		const (
			testAppGUID = "unused-app-guid"
		)

		var buildLogs, appLogs []repositories.LogRecord

		BeforeEach(func() {
			var err error
			req, err = http.NewRequestWithContext(ctx, "GET", "/api/v1/read/"+testAppGUID, nil)
			Expect(err).NotTo(HaveOccurred())

			buildLogs = []repositories.LogRecord{
				{
					Message:   "BuildMessage1",
					Timestamp: time.Now().UnixNano(),
					Tags: map[string]string{
						"source_type": "STG",
					},
				},
				{
					Message:   "BuildMessage2",
					Timestamp: time.Now().UnixNano(),
					Tags: map[string]string{
						"source_type": "STG",
					},
				},
			}

			time.Sleep(5 * time.Millisecond)

			appLogs = []repositories.LogRecord{
				{
					Message:   "AppMessage1",
					Timestamp: time.Now().UnixNano(),
				},
				{
					Message:   "AppMessage2",
					Timestamp: time.Now().UnixNano(),
				},
			}
			appLogsReader.ReadReturns(append(buildLogs, appLogs...), nil)
		})

		It("returns status 200 OK", func() {
			Expect(rr.Code).To(Equal(http.StatusOK))
		})

		It("returns Content-Type as JSON in header", func() {
			contentTypeHeader := rr.Header().Get("Content-Type")
			Expect(contentTypeHeader).To(Equal(jsonHeader))
		})

		It("returns a list of log envelopes", func() {
			expectedBody := fmt.Sprintf(`{
				"envelopes": {
					"batch": [
						{
							"timestamp": %[1]d,
							"log": {
								"payload": "%[2]s",
								"type": 0
							},
							"tags": {
								"source_type": "STG"
							}
						},
						{
							"timestamp": %[3]d,
							"log": {
								"payload": "%[4]s",
								"type": 0
							},
							"tags": {
								"source_type": "STG"
							}
						},
						{
							"timestamp": %[5]d,
							"log": {
								"payload": "%[6]s",
								"type": 0
							}
						},
						{
							"timestamp": %[7]d,
							"log": {
								"payload": "%[8]s",
								"type": 0
							}
						}
					]
				}
			}`, buildLogs[0].Timestamp, base64.URLEncoding.EncodeToString([]byte(buildLogs[0].Message)), buildLogs[1].Timestamp, base64.URLEncoding.EncodeToString([]byte(buildLogs[1].Message)),
				appLogs[0].Timestamp, base64.URLEncoding.EncodeToString([]byte(appLogs[0].Message)), appLogs[1].Timestamp, base64.URLEncoding.EncodeToString([]byte(appLogs[1].Message)))
			Expect(rr.Body).To(MatchJSON(expectedBody))
		})

		When("query parameters are specified", func() {
			BeforeEach(func() {
				var err error
				req, err = http.NewRequestWithContext(ctx, "GET", "/api/v1/read/"+testAppGUID+"?descending=true&envelope_types=LOG&envelope_types=TIMER&limit=1000&start_time=-6795364578871345152", nil)
				Expect(err).NotTo(HaveOccurred())
			})

			It("returns status 200 OK", func() {
				Expect(rr.Code).Should(Equal(http.StatusOK))
			})
		})

		When("an invalid envelope type is provided", func() {
			BeforeEach(func() {
				var err error
				req, err = http.NewRequestWithContext(ctx, "GET", "/api/v1/read/"+testAppGUID+"?envelope_types=BAD", nil)
				Expect(err).NotTo(HaveOccurred())
			})

			It("returns an Unknown key error", func() {
				expectUnprocessableEntityError("error validating log read query parameters")
			})
		})

		When("an invalid envelope type is provided#2", func() {
			BeforeEach(func() {
				var err error
				// Commas are literally interpreted instead of automatically placed as a list
				req, err = http.NewRequestWithContext(ctx, "GET", "/api/v1/read/"+testAppGUID+"?envelope_types=LOG,TIMER", nil)
				Expect(err).NotTo(HaveOccurred())
			})

			It("returns an Unknown key error", func() {
				expectUnprocessableEntityError("error validating log read query parameters")
			})
		})

		When("invalid query parameters are provided", func() {
			BeforeEach(func() {
				var err error
				req, err = http.NewRequestWithContext(ctx, "GET", "/api/v1/read/"+testAppGUID+"?foo=bar", nil)
				Expect(err).NotTo(HaveOccurred())
			})

			It("returns an Unknown key error", func() {
				expectUnknownKeyError("The query parameter is invalid: Valid parameters are: 'start_time, end_time, envelope_types, limit, descending'")
			})
		})

		When("the action returns a not-found error", func() {
			BeforeEach(func() {
				appLogsReader.ReadReturns(nil, apierrors.NewNotFoundError(nil, repositories.AppResourceType))
			})
			It("elevates the error", func() {
				expectNotFoundError("App not found")
			})
		})

		When("the action returns a random error", func() {
			BeforeEach(func() {
				appLogsReader.ReadReturns(nil, errors.New("i-am-made-up"))
			})
			It("returns an Unknown error", func() {
				expectUnknownError()
			})
		})
	})
})
