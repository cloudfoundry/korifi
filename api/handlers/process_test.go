package handlers_test

import (
	"errors"
	"io"
	"net/http"
	"strings"

	"code.cloudfoundry.org/korifi/api/actions"
	apierrors "code.cloudfoundry.org/korifi/api/errors"
	. "code.cloudfoundry.org/korifi/api/handlers"
	"code.cloudfoundry.org/korifi/api/handlers/fake"
	"code.cloudfoundry.org/korifi/api/payloads"
	"code.cloudfoundry.org/korifi/api/repositories"
	. "code.cloudfoundry.org/korifi/tests/matchers"
	"code.cloudfoundry.org/korifi/tools"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Process", func() {
	var (
		processRepo      *fake.CFProcessRepository
		processStats     *fake.ProcessStats
		requestValidator *fake.RequestValidator
	)

	BeforeEach(func() {
		processRepo = new(fake.CFProcessRepository)
		processStats = new(fake.ProcessStats)
		requestValidator = new(fake.RequestValidator)

		apiHandler := NewProcess(
			*serverURL,
			processRepo,
			processStats,
			requestValidator,
		)
		routerBuilder.LoadRoutes(apiHandler)
	})

	Describe("the GET /v3/processes/:guid endpoint", func() {
		BeforeEach(func() {
			processRepo.GetProcessReturns(repositories.ProcessRecord{
				GUID: "process-guid",
			}, nil)
		})

		JustBeforeEach(func() {
			req, err := http.NewRequestWithContext(ctx, "GET", "/v3/processes/process-guid", nil)
			Expect(err).NotTo(HaveOccurred())
			routerBuilder.Build().ServeHTTP(rr, req)
		})

		It("returns a process", func() {
			Expect(processRepo.GetProcessCallCount()).To(Equal(1))
			_, actualAuthInfo, actualProcessGUID := processRepo.GetProcessArgsForCall(0)
			Expect(actualAuthInfo).To(Equal(authInfo))
			Expect(actualProcessGUID).To(Equal("process-guid"))

			Expect(rr).To(HaveHTTPStatus(http.StatusOK))
			Expect(rr).To(HaveHTTPHeaderWithValue("Content-Type", "application/json"))

			Expect(rr).To(HaveHTTPBody(SatisfyAll(
				MatchJSONPath("$.guid", "process-guid"),
				MatchJSONPath("$.links.self.href", "https://api.example.org/v3/processes/process-guid"),
			)))
		})

		When("the user lacks access", func() {
			BeforeEach(func() {
				processRepo.GetProcessReturns(repositories.ProcessRecord{}, apierrors.NewForbiddenError(errors.New("access denied or something"), repositories.ProcessResourceType))
			})

			It("returns a not-found error", func() {
				expectNotFoundError("Process")
			})
		})

		When("there is some other error fetching the process", func() {
			BeforeEach(func() {
				processRepo.GetProcessReturns(repositories.ProcessRecord{}, errors.New("unknown!"))
			})

			It("returns an error", func() {
				expectUnknownError()
			})
		})
	})

	Describe("the GET /v3/processes/:guid/sidecars endpoint", func() {
		BeforeEach(func() {
			processRepo.GetProcessReturns(repositories.ProcessRecord{}, nil)
		})

		JustBeforeEach(func() {
			req, err := http.NewRequestWithContext(ctx, "GET", "/v3/processes/process-guid/sidecars", nil)
			Expect(err).NotTo(HaveOccurred())
			routerBuilder.Build().ServeHTTP(rr, req)
		})

		It("returns the empty list of sidecars", func() {
			Expect(processRepo.GetProcessCallCount()).To(Equal(1))
			_, actualAuthInfo, _ := processRepo.GetProcessArgsForCall(0)
			Expect(actualAuthInfo).To(Equal(authInfo))

			Expect(rr).To(HaveHTTPStatus(http.StatusOK))
			Expect(rr).To(HaveHTTPHeaderWithValue("Content-Type", "application/json"))
			Expect(rr).To(HaveHTTPBody(SatisfyAll(
				MatchJSONPath("$.pagination.total_results", BeZero()),
				MatchJSONPath("$.pagination.first.href", "https://api.example.org/v3/processes/process-guid/sidecars"),
			)))
		})

		When("the process isn't accessible to the user", func() {
			BeforeEach(func() {
				processRepo.GetProcessReturns(repositories.ProcessRecord{}, apierrors.NewForbiddenError(nil, repositories.ProcessResourceType))
			})

			It("returns an error", func() {
				expectNotFoundError("Process")
			})
		})

		When("there is some other error fetching the process", func() {
			BeforeEach(func() {
				processRepo.GetProcessReturns(repositories.ProcessRecord{}, errors.New("unknown!"))
			})

			It("returns an error", func() {
				expectUnknownError()
			})
		})
	})

	Describe("the POST /v3/processes/:guid/actions/scale endpoint", func() {
		var requestBody string

		BeforeEach(func() {
			processRepo.GetProcessReturns(repositories.ProcessRecord{
				GUID:      "process-guid",
				SpaceGUID: spaceGUID,
			}, nil)

			processRepo.ScaleProcessReturns(repositories.ProcessRecord{
				GUID: "process-guid",
			}, nil)

			requestBody = `{
				"instances": 3,
				"memory_in_mb": 512,
				"disk_in_mb": 256
			}`

			requestValidator.DecodeAndValidateJSONPayloadStub = decodeAndValidateJSONPayloadStub(&payloads.ProcessScale{
				Instances: tools.PtrTo(3),
				MemoryMB:  tools.PtrTo[int64](512),
				DiskMB:    tools.PtrTo[int64](256),
			})
		})

		JustBeforeEach(func() {
			req, err := http.NewRequestWithContext(ctx, "POST", "/v3/processes/process-guid/actions/scale", strings.NewReader(requestBody))
			Expect(err).NotTo(HaveOccurred())
			routerBuilder.Build().ServeHTTP(rr, req)
		})

		It("scales the process", func() {
			Expect(requestValidator.DecodeAndValidateJSONPayloadCallCount()).To(Equal(1))
			req, _ := requestValidator.DecodeAndValidateJSONPayloadArgsForCall(0)
			reqBytes, err := io.ReadAll(req.Body)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(reqBytes)).To(Equal(requestBody))

			Expect(processRepo.GetProcessCallCount()).To(Equal(1))
			_, actualAuthInfo, actualProcessGUID := processRepo.GetProcessArgsForCall(0)
			Expect(actualAuthInfo).To(Equal(authInfo))
			Expect(actualProcessGUID).To(Equal("process-guid"))

			Expect(processRepo.ScaleProcessCallCount()).To(Equal(1))
			_, actualAuthInfo, scaleProcessMsg := processRepo.ScaleProcessArgsForCall(0)
			Expect(actualAuthInfo).To(Equal(authInfo))
			Expect(scaleProcessMsg).To(Equal(repositories.ScaleProcessMessage{
				GUID:      "process-guid",
				SpaceGUID: spaceGUID,
				ProcessScaleValues: repositories.ProcessScaleValues{
					Instances: tools.PtrTo(3),
					MemoryMB:  tools.PtrTo(int64(512)),
					DiskMB:    tools.PtrTo(int64(256)),
				},
			}))
		})

		It("scales the process", func() {
			Expect(rr).To(HaveHTTPStatus(http.StatusOK))
			Expect(rr).To(HaveHTTPHeaderWithValue("Content-Type", "application/json"))
			Expect(rr).To(HaveHTTPBody(SatisfyAll(
				MatchJSONPath("$.guid", "process-guid"),
				MatchJSONPath("$.links.self.href", "https://api.example.org/v3/processes/process-guid"),
			)))
		})

		When("the request JSON is invalid", func() {
			BeforeEach(func() {
				requestValidator.DecodeAndValidateJSONPayloadReturns(errors.New("boom"))
			})

			It("returns an error", func() {
				expectUnknownError()
			})
		})

		When("the user does not have permissions to get the process", func() {
			BeforeEach(func() {
				processRepo.GetProcessReturns(repositories.ProcessRecord{}, apierrors.NewForbiddenError(nil, "Process"))
			})

			It("returns a NotFound error", func() {
				expectNotFoundError("Process")
			})
		})

		When("getting the process errors", func() {
			BeforeEach(func() {
				processRepo.GetProcessReturns(repositories.ProcessRecord{}, errors.New("unknown!"))
			})

			It("returns an error", func() {
				expectUnknownError()
			})
		})

		When("scaling errors", func() {
			BeforeEach(func() {
				processRepo.ScaleProcessReturns(repositories.ProcessRecord{}, errors.New("unknown!"))
			})

			It("returns an error", func() {
				expectUnknownError()
			})
		})
	})

	Describe("the GET /v3/processes/<guid>/stats endpoint", func() {
		BeforeEach(func() {
			processStats.FetchStatsReturns([]actions.PodStatsRecord{
				{
					Type:     "web",
					Index:    0,
					MemQuota: tools.PtrTo(int64(1024)),
				},
				{
					Type:     "web",
					Index:    1,
					MemQuota: tools.PtrTo(int64(512)),
				},
			}, nil)
		})

		JustBeforeEach(func() {
			req, err := http.NewRequestWithContext(ctx, "GET", "/v3/processes/process-guid/stats", nil)
			Expect(err).NotTo(HaveOccurred())
			routerBuilder.Build().ServeHTTP(rr, req)
		})

		It("returns the process stats", func() {
			Expect(processStats.FetchStatsCallCount()).To(Equal(1))
			_, actualAuthInfo, _ := processStats.FetchStatsArgsForCall(0)
			Expect(actualAuthInfo).To(Equal(authInfo))

			Expect(rr).To(HaveHTTPStatus(http.StatusOK))
			Expect(rr).To(HaveHTTPHeaderWithValue("Content-Type", "application/json"))

			Expect(rr).To(HaveHTTPBody(SatisfyAll(
				MatchJSONPath("$.resources", HaveLen(2)),
				MatchJSONPath("$.resources[0].type", "web"),
				MatchJSONPath("$.resources[0].index", BeEquivalentTo(0)),
				MatchJSONPath("$.resources[1].type", "web"),
				MatchJSONPath("$.resources[1].mem_quota", BeEquivalentTo(512)),
			)))
		})

		When("fetching stats fails with an unauthorized error", func() {
			BeforeEach(func() {
				processStats.FetchStatsReturns(nil, apierrors.NewForbiddenError(nil, repositories.AppResourceType))
			})

			It("returns a not found error", func() {
				expectNotFoundError("App")
			})
		})

		When("fetching the process stats errors", func() {
			BeforeEach(func() {
				processStats.FetchStatsReturns(nil, errors.New("boom"))
			})

			It("returns an error", func() {
				expectUnknownError()
			})
		})
	})

	Describe("the GET /v3/processes endpoint", func() {
		BeforeEach(func() {
			requestValidator.DecodeAndValidateURLValuesStub = decodeAndValidateURLValuesStub(&payloads.ProcessList{})
			processRepo.ListProcessesReturns([]repositories.ProcessRecord{
				{
					GUID: "process-guid",
				},
			}, nil)
		})

		JustBeforeEach(func() {
			req, err := http.NewRequestWithContext(ctx, "GET", "/v3/processes", nil)
			Expect(err).NotTo(HaveOccurred())
			routerBuilder.Build().ServeHTTP(rr, req)
		})

		It("returns the processes", func() {
			Expect(requestValidator.DecodeAndValidateURLValuesCallCount()).To(Equal(1))
			req, _ := requestValidator.DecodeAndValidateURLValuesArgsForCall(0)
			Expect(req.URL.String()).To(HaveSuffix("/v3/processes"))

			Expect(rr).To(HaveHTTPStatus(http.StatusOK))
			Expect(rr).To(HaveHTTPHeaderWithValue("Content-Type", "application/json"))
			Expect(rr).To(HaveHTTPBody(SatisfyAll(
				MatchJSONPath("$.pagination.total_results", BeEquivalentTo(1)),
				MatchJSONPath("$.pagination.first.href", "https://api.example.org/v3/processes"),
				MatchJSONPath("$.resources[0].guid", "process-guid"),
			)))
		})

		When("app_guids query parameter is provided", func() {
			BeforeEach(func() {
				requestValidator.DecodeAndValidateURLValuesStub = decodeAndValidateURLValuesStub(&payloads.ProcessList{
					AppGUIDs: "my-app-guid",
				})
			})

			It("invokes process repository with correct args", func() {
				_, _, message := processRepo.ListProcessesArgsForCall(0)
				Expect(message.AppGUIDs).To(HaveLen(1))
				Expect(message.AppGUIDs[0]).To(Equal("my-app-guid"))

				Expect(rr).To(HaveHTTPStatus(http.StatusOK))
			})
		})

		When("the request body is invalid", func() {
			BeforeEach(func() {
				requestValidator.DecodeAndValidateURLValuesReturns(errors.New("boo"))
			})

			It("returns an error", func() {
				expectUnknownError()
			})
		})

		When("listing processes fails", func() {
			BeforeEach(func() {
				processRepo.ListProcessesReturns(nil, errors.New("boom"))
			})

			It("returns an error", func() {
				expectUnknownError()
			})
		})
	})

	Describe("the PATCH /v3/processes/:guid endpoint", func() {
		var requestBody string

		BeforeEach(func() {
			requestBody = `{
				"health_check": {
					"data": {
						"invocation_timeout": 2,
						"timeout": 5,
						"endpoint": "http://myapp.com/health"
					},
					"type": "port"
				},
				"metadata": {
					"labels": {
						"foo": "value1"
					}
				}
			}`

			processRepo.GetProcessReturns(repositories.ProcessRecord{
				GUID: "process-guid",
			}, nil)

			processRepo.PatchProcessReturns(repositories.ProcessRecord{
				GUID: "process-guid",
			}, nil)

			requestValidator.DecodeAndValidateJSONPayloadStub = decodeAndValidateJSONPayloadStub(&payloads.ProcessPatch{
				Metadata: &payloads.MetadataPatch{
					Labels: map[string]*string{
						"foo": tools.PtrTo("value1"),
					},
				},
				HealthCheck: &payloads.HealthCheck{
					Type: tools.PtrTo("port"),
					Data: &payloads.Data{
						Timeout:           tools.PtrTo[int64](5),
						Endpoint:          tools.PtrTo("http://myapp.com/health"),
						InvocationTimeout: tools.PtrTo[int64](2),
					},
				},
			})
		})

		JustBeforeEach(func() {
			req, err := http.NewRequestWithContext(ctx, "PATCH", "/v3/processes/process-guid", strings.NewReader(requestBody))
			Expect(err).NotTo(HaveOccurred())
			routerBuilder.Build().ServeHTTP(rr, req)
		})

		It("updates the process", func() {
			Expect(requestValidator.DecodeAndValidateJSONPayloadCallCount()).To(Equal(1))
			req, _ := requestValidator.DecodeAndValidateJSONPayloadArgsForCall(0)
			reqBytes, err := io.ReadAll(req.Body)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(reqBytes)).To(Equal(requestBody))

			Expect(processRepo.PatchProcessCallCount()).To(Equal(1))
			_, actualAuthInfo, actualMsg := processRepo.PatchProcessArgsForCall(0)
			Expect(actualAuthInfo).To(Equal(authInfo))
			Expect(actualMsg.ProcessGUID).To(Equal("process-guid"))
			Expect(actualMsg.HealthCheckInvocationTimeoutSeconds).To(Equal(tools.PtrTo(int64(2))))
			Expect(actualMsg.HealthCheckTimeoutSeconds).To(Equal(tools.PtrTo(int64(5))))
			Expect(actualMsg.HealthCheckHTTPEndpoint).To(Equal(tools.PtrTo("http://myapp.com/health")))
			Expect(actualMsg.HealthCheckType).To(Equal(tools.PtrTo("port")))
			Expect(actualMsg.MetadataPatch.Labels).To(Equal(map[string]*string{"foo": tools.PtrTo("value1")}))

			Expect(rr).To(HaveHTTPStatus(http.StatusOK))
			Expect(rr).To(HaveHTTPHeaderWithValue("Content-Type", "application/json"))
			Expect(rr).To(HaveHTTPBody(SatisfyAll(
				MatchJSONPath("$.guid", "process-guid"),
				MatchJSONPath("$.links.self.href", "https://api.example.org/v3/processes/process-guid"),
			)))
		})

		When("the request body is invalid json", func() {
			BeforeEach(func() {
				requestValidator.DecodeAndValidateJSONPayloadReturns(errors.New("boom"))
			})

			It("return an error", func() {
				expectUnknownError()
			})
		})

		When("user is not allowed to get a process", func() {
			BeforeEach(func() {
				processRepo.GetProcessReturns(repositories.ProcessRecord{}, apierrors.NewForbiddenError(errors.New("nope"), repositories.ProcessResourceType))
			})

			It("returns a not found error", func() {
				expectNotFoundError("Process")
			})
		})

		When("getting the process fails a process", func() {
			BeforeEach(func() {
				processRepo.GetProcessReturns(repositories.ProcessRecord{}, errors.New("boom"))
			})

			It("returns an error", func() {
				expectUnknownError()
			})
		})

		When("patching the process fails a process", func() {
			BeforeEach(func() {
				processRepo.PatchProcessReturns(repositories.ProcessRecord{}, errors.New("boom"))
			})

			It("returns an error", func() {
				expectUnknownError()
			})
		})
	})
})
