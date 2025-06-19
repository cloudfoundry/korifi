package handlers_test

import (
	"errors"
	"net/http"
	"strings"

	apierrors "code.cloudfoundry.org/korifi/api/errors"
	. "code.cloudfoundry.org/korifi/api/handlers"
	"code.cloudfoundry.org/korifi/api/handlers/fake"
	"code.cloudfoundry.org/korifi/api/handlers/stats"
	"code.cloudfoundry.org/korifi/api/payloads"
	"code.cloudfoundry.org/korifi/api/repositories"
	"code.cloudfoundry.org/korifi/api/repositories/k8sklient/descriptors"
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	. "code.cloudfoundry.org/korifi/tests/matchers"
	"code.cloudfoundry.org/korifi/tools"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Process", func() {
	var (
		processRepo             *fake.CFProcessRepository
		requestValidator        *fake.RequestValidator
		podRepo                 *fake.PodRepository
		gaugesCollector         *fake.GaugesCollector
		instancesStateCollector *fake.InstancesStateCollector
	)

	BeforeEach(func() {
		processRepo = new(fake.CFProcessRepository)
		requestValidator = new(fake.RequestValidator)
		podRepo = new(fake.PodRepository)
		gaugesCollector = new(fake.GaugesCollector)
		instancesStateCollector = new(fake.InstancesStateCollector)

		apiHandler := NewProcess(
			*serverURL,
			processRepo,
			requestValidator,
			podRepo,
			gaugesCollector,
			instancesStateCollector,
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
		BeforeEach(func() {
			processRepo.GetProcessReturns(repositories.ProcessRecord{
				GUID:      "process-guid",
				SpaceGUID: spaceGUID,
			}, nil)

			processRepo.ScaleProcessReturns(repositories.ProcessRecord{
				GUID: "process-guid",
			}, nil)

			requestValidator.DecodeAndValidateJSONPayloadStub = decodeAndValidatePayloadStub(&payloads.ProcessScale{
				Instances: tools.PtrTo[int32](3),
				MemoryMB:  tools.PtrTo[int64](512),
				DiskMB:    tools.PtrTo[int64](256),
			})
		})

		JustBeforeEach(func() {
			req, err := http.NewRequestWithContext(ctx, "POST", "/v3/processes/process-guid/actions/scale", strings.NewReader("the-json-body"))
			Expect(err).NotTo(HaveOccurred())
			routerBuilder.Build().ServeHTTP(rr, req)
		})

		It("scales the process", func() {
			Expect(requestValidator.DecodeAndValidateJSONPayloadCallCount()).To(Equal(1))
			actualReq, _ := requestValidator.DecodeAndValidateJSONPayloadArgsForCall(0)
			Expect(bodyString(actualReq)).To(Equal("the-json-body"))

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
					Instances: tools.PtrTo[int32](3),
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
			processRepo.GetProcessReturns(repositories.ProcessRecord{
				GUID:    "process-guid",
				AppGUID: "app-guid",
			}, nil)

			gaugesCollector.CollectProcessGaugesReturns([]stats.ProcessGauges{{
				Index:    0,
				MemQuota: tools.PtrTo(int64(1024)),
			}, {
				Index:    1,
				MemQuota: tools.PtrTo(int64(512)),
			}}, nil)

			instancesStateCollector.CollectProcessInstancesStatesReturns([]stats.ProcessInstanceState{
				{
					ID:    0,
					Type:  "web",
					State: korifiv1alpha1.InstanceStateRunning,
				},
				{
					ID:    1,
					Type:  "web",
					State: korifiv1alpha1.InstanceStateDown,
				},
			}, nil)
		})

		JustBeforeEach(func() {
			req, err := http.NewRequestWithContext(ctx, "GET", "/v3/processes/process-guid/stats", nil)
			Expect(err).NotTo(HaveOccurred())
			routerBuilder.Build().ServeHTTP(rr, req)
		})

		It("returns the process stats", func() {
			Expect(processRepo.GetProcessCallCount()).To(Equal(1))
			_, actualAuthInfo, actualProcessGUID := processRepo.GetProcessArgsForCall(0)
			Expect(actualAuthInfo).To(Equal(authInfo))
			Expect(actualProcessGUID).To(Equal("process-guid"))

			Expect(gaugesCollector.CollectProcessGaugesCallCount()).To(Equal(1))
			_, actualAppGUID, actualProcessGUID := gaugesCollector.CollectProcessGaugesArgsForCall(0)
			Expect(actualAppGUID).To(Equal("app-guid"))
			Expect(actualProcessGUID).To(Equal("process-guid"))

			Expect(instancesStateCollector.CollectProcessInstancesStatesCallCount()).To(Equal(1))
			_, actualProcessGUID = instancesStateCollector.CollectProcessInstancesStatesArgsForCall(0)
			Expect(actualProcessGUID).To(Equal("process-guid"))

			Expect(rr).To(HaveHTTPStatus(http.StatusOK))
			Expect(rr).To(HaveHTTPHeaderWithValue("Content-Type", "application/json"))

			Expect(rr).To(HaveHTTPBody(SatisfyAll(
				MatchJSONPath("$.resources", HaveLen(2)),
				MatchJSONPath("$.resources[0].type", "web"),
				MatchJSONPath("$.resources[0].index", BeEquivalentTo(0)),
				MatchJSONPath("$.resources[0].mem_quota", BeEquivalentTo(1024)),
				MatchJSONPath("$.resources[0].state", Equal("RUNNING")),
				MatchJSONPath("$.resources[1].type", "web"),
				MatchJSONPath("$.resources[1].index", BeEquivalentTo(1)),
				MatchJSONPath("$.resources[1].mem_quota", BeEquivalentTo(512)),
				MatchJSONPath("$.resources[1].state", Equal("DOWN")),
			)))
		})

		When("getting the process fails with forbidden error", func() {
			BeforeEach(func() {
				processRepo.GetProcessReturns(repositories.ProcessRecord{}, apierrors.NewForbiddenError(nil, repositories.ProcessResourceType))
			})

			It("returns a not found error", func() {
				expectNotFoundError("Process")
			})
		})

		When("getting the process fails", func() {
			BeforeEach(func() {
				processRepo.GetProcessReturns(repositories.ProcessRecord{}, errors.New("boom"))
			})

			It("returns an error", func() {
				expectUnknownError()
			})
		})

		When("collecting instance state fails", func() {
			BeforeEach(func() {
				instancesStateCollector.CollectProcessInstancesStatesReturns(nil, errors.New("boom"))
			})

			It("returns an error", func() {
				expectUnknownError()
			})
		})

		When("collecting gauges fails", func() {
			BeforeEach(func() {
				gaugesCollector.CollectProcessGaugesReturns(nil, errors.New("boom"))
			})

			It("returns an error", func() {
				expectUnknownError()
			})
		})
	})

	Describe("the GET /v3/processes endpoint", func() {
		BeforeEach(func() {
			requestValidator.DecodeAndValidateURLValuesStub = decodeAndValidateURLValuesStub(&payloads.ProcessList{})
			processRepo.ListProcessesReturns(repositories.ListResult[repositories.ProcessRecord]{
				Records: []repositories.ProcessRecord{{
					GUID: "process-guid",
				}},
				PageInfo: descriptors.PageInfo{
					TotalResults: 1,
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
				processRepo.ListProcessesReturns(repositories.ListResult[repositories.ProcessRecord]{}, errors.New("boom"))
			})

			It("returns an error", func() {
				expectUnknownError()
			})
		})
	})

	Describe("the PATCH /v3/processes/:guid endpoint", func() {
		BeforeEach(func() {
			processRepo.GetProcessReturns(repositories.ProcessRecord{
				GUID: "process-guid",
			}, nil)

			processRepo.PatchProcessReturns(repositories.ProcessRecord{
				GUID: "process-guid",
			}, nil)

			requestValidator.DecodeAndValidateJSONPayloadStub = decodeAndValidatePayloadStub(&payloads.ProcessPatch{
				Metadata: &payloads.MetadataPatch{
					Labels: map[string]*string{
						"foo": tools.PtrTo("value1"),
					},
				},
				HealthCheck: &payloads.HealthCheck{
					Type: tools.PtrTo("port"),
					Data: &payloads.Data{
						Timeout:           tools.PtrTo[int32](5),
						Endpoint:          tools.PtrTo("http://myapp.com/health"),
						InvocationTimeout: tools.PtrTo[int32](2),
					},
				},
			})
		})

		JustBeforeEach(func() {
			req, err := http.NewRequestWithContext(ctx, "PATCH", "/v3/processes/process-guid", strings.NewReader("the-json-body"))
			Expect(err).NotTo(HaveOccurred())
			routerBuilder.Build().ServeHTTP(rr, req)
		})

		It("updates the process", func() {
			Expect(requestValidator.DecodeAndValidateJSONPayloadCallCount()).To(Equal(1))
			actualReq, _ := requestValidator.DecodeAndValidateJSONPayloadArgsForCall(0)
			Expect(bodyString(actualReq)).To(Equal("the-json-body"))

			Expect(processRepo.PatchProcessCallCount()).To(Equal(1))
			_, actualAuthInfo, actualMsg := processRepo.PatchProcessArgsForCall(0)
			Expect(actualAuthInfo).To(Equal(authInfo))
			Expect(actualMsg.ProcessGUID).To(Equal("process-guid"))
			Expect(actualMsg.HealthCheckInvocationTimeoutSeconds).To(Equal(tools.PtrTo(int32(2))))
			Expect(actualMsg.HealthCheckTimeoutSeconds).To(Equal(tools.PtrTo(int32(5))))
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

	Describe("DELETE /v3/processes/:guid/instances/:instance", func() {
		var instance string

		BeforeEach(func() {
			processRepo.GetProcessReturns(repositories.ProcessRecord{
				GUID:             "process-guid",
				AppGUID:          "app-guid",
				SpaceGUID:        "space-guid",
				DesiredInstances: 2,
				Type:             "web",
			}, nil)
			processRepo.GetAppRevisionReturns("0", nil)
			instance = "0"
		})

		JustBeforeEach(func() {
			req, err := http.NewRequestWithContext(ctx, "DELETE", "/v3/processes/process-guid/instances/"+instance, strings.NewReader("the-json-body"))
			Expect(err).NotTo(HaveOccurred())
			routerBuilder.Build().ServeHTTP(rr, req)
		})

		It("restarts the instance", func() {
			Expect(rr).To(HaveHTTPStatus(http.StatusNoContent))
			Expect(podRepo.DeletePodCallCount()).To(Equal(1))
			_, actualAuthInfo, actualAppRevision, actualProcess, actualInstanceID := podRepo.DeletePodArgsForCall(0)
			Expect(actualAuthInfo).To(Equal(authInfo))
			Expect(actualAppRevision).To(Equal("0"))
			Expect(actualProcess.AppGUID).To(Equal("app-guid"))
			Expect(actualProcess.SpaceGUID).To(Equal("space-guid"))
			Expect(actualProcess.Type).To(Equal("web"))
			Expect(actualInstanceID).To(Equal("0"))
		})

		When("the instance is not found", func() {
			BeforeEach(func() {
				instance = "5"
			})

			It("returns an error", func() {
				expectNotFoundError("Instance 5 of process web")
			})
		})

		When("the process is not found", func() {
			BeforeEach(func() {
				processRepo.GetProcessReturns(repositories.ProcessRecord{}, apierrors.NewForbiddenError(errors.New("access denied or something"), repositories.ProcessResourceType))
			})

			It("returns a not found error", func() {
				expectNotFoundError("Process")
			})
		})
	})
})
