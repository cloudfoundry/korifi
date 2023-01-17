package handlers_test

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"

	"code.cloudfoundry.org/korifi/api/actions"
	apierrors "code.cloudfoundry.org/korifi/api/errors"
	. "code.cloudfoundry.org/korifi/api/handlers"
	"code.cloudfoundry.org/korifi/api/handlers/fake"
	"code.cloudfoundry.org/korifi/api/repositories"
	"code.cloudfoundry.org/korifi/tools"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Process", func() {
	const (
		processGUID = "test-process-guid"
	)

	var (
		processRepo   *fake.CFProcessRepository
		processStats  *fake.ProcessStats
		processScaler *fake.ProcessScaler
		req           *http.Request
	)

	BeforeEach(func() {
		processRepo = new(fake.CFProcessRepository)
		processStats = new(fake.ProcessStats)
		processScaler = new(fake.ProcessScaler)
		decoderValidator, err := NewDefaultDecoderValidator()
		Expect(err).NotTo(HaveOccurred())

		apiHandler := NewProcess(
			*serverURL,
			processRepo,
			processStats,
			processScaler,
			decoderValidator,
		)
		routerBuilder.LoadRoutes(apiHandler)
	})

	JustBeforeEach(func() {
		routerBuilder.Build().ServeHTTP(rr, req)
	})

	Describe("the GET /v3/processes/:guid endpoint", func() {
		const (
			processGUID     = "process-guid"
			spaceGUID       = "space-guid"
			appGUID         = "app-guid"
			createdAt       = "1906-04-18T13:12:00Z"
			updatedAt       = "1906-04-18T13:12:01Z"
			processType     = "web"
			command         = "bundle exec rackup config.ru -p $PORT -o 0.0.0.0"
			memoryInMB      = 256
			diskInMB        = 1024
			healthcheckType = "port"
			instances       = 1

			baseURL = "https://api.example.org"
		)

		var (
			labels      = map[string]string{}
			annotations = map[string]string{}
		)

		BeforeEach(func() {
			processRepo.GetProcessReturns(repositories.ProcessRecord{
				GUID:             processGUID,
				SpaceGUID:        spaceGUID,
				AppGUID:          appGUID,
				CreatedAt:        createdAt,
				UpdatedAt:        updatedAt,
				Type:             processType,
				Command:          command,
				DesiredInstances: instances,
				MemoryMB:         memoryInMB,
				DiskQuotaMB:      diskInMB,
				HealthCheck: repositories.HealthCheck{
					Type: healthcheckType,
					Data: repositories.HealthCheckData{},
				},
				Labels:      labels,
				Annotations: annotations,
			}, nil)

			var err error
			req, err = http.NewRequestWithContext(ctx, "GET", "/v3/processes/"+processGUID, nil)
			Expect(err).NotTo(HaveOccurred())
		})

		When("on the happy path", func() {
			It("returns status 200 OK", func() {
				Expect(rr.Code).To(Equal(http.StatusOK), "Matching HTTP response code:")
			})

			It("passes the authorization.Info to the process repository", func() {
				Expect(processRepo.GetProcessCallCount()).To(Equal(1))
				_, actualAuthInfo, _ := processRepo.GetProcessArgsForCall(0)
				Expect(actualAuthInfo).To(Equal(authInfo))
			})

			It("returns a process", func() {
				contentTypeHeader := rr.Header().Get("Content-Type")
				Expect(contentTypeHeader).To(Equal(jsonHeader), "Matching Content-Type header:")

				Expect(rr.Body.String()).To(MatchJSON(`{
					"guid": "` + processGUID + `",
					"created_at": "` + createdAt + `",
					"updated_at": "` + updatedAt + `",
					"type": "web",
					"command": "bundle exec rackup config.ru -p $PORT -o 0.0.0.0",
					"instances": ` + fmt.Sprint(instances) + `,
					"memory_in_mb": ` + fmt.Sprint(memoryInMB) + `,
					"disk_in_mb": ` + fmt.Sprint(diskInMB) + `,
					"health_check": {
					   "type": "` + healthcheckType + `",
					   "data": {
						  "timeout": null,
						  "invocation_timeout": null
					   }
					},
					"relationships": {
					   "app": {
						  "data": {
							 "guid": "` + appGUID + `"
						  }
					   }
					},
					"metadata": {
					   "labels": {},
					   "annotations": {}
					},
					"links": {
					   "self": {
						  "href": "` + baseURL + `/v3/processes/` + processGUID + `"
					   },
					   "scale": {
						  "href": "` + baseURL + `/v3/processes/` + processGUID + `/actions/scale",
						  "method": "POST"
					   },
					   "app": {
						  "href": "` + baseURL + `/v3/apps/` + appGUID + `"
					   },
					   "space": {
						  "href": "` + baseURL + `/v3/spaces/` + spaceGUID + `"
					   },
					   "stats": {
						  "href": "` + baseURL + `/v3/processes/` + processGUID + `/stats"
					   }
					}
				 }`))
			})
		})

		When("on the sad path and", func() {
			When("the user lacks access", func() {
				BeforeEach(func() {
					processRepo.GetProcessReturns(repositories.ProcessRecord{}, apierrors.NewForbiddenError(errors.New("access denied or something"), repositories.ProcessResourceType))
				})

				It("returns a not-found error", func() {
					expectNotFoundError("Process not found")
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
	})

	Describe("the GET /v3/processes/:guid/sidecars endpoint", func() {
		BeforeEach(func() {
			processRepo.GetProcessReturns(repositories.ProcessRecord{}, nil)

			var err error
			req, err = http.NewRequestWithContext(ctx, "GET", "/v3/processes/"+processGUID+"/sidecars", nil)
			Expect(err).NotTo(HaveOccurred())
		})

		When("on the happy path", func() {
			It("returns status 200 OK", func() {
				Expect(rr.Code).To(Equal(http.StatusOK), "Matching HTTP response code:")
			})

			It("passes the authorization.Info to the process repository", func() {
				Expect(processRepo.GetProcessCallCount()).To(Equal(1))
				_, actualAuthInfo, _ := processRepo.GetProcessArgsForCall(0)
				Expect(actualAuthInfo).To(Equal(authInfo))
			})

			It("returns a canned response with the processGUID in it", func() {
				contentTypeHeader := rr.Header().Get("Content-Type")
				Expect(contentTypeHeader).To(Equal(jsonHeader), "Matching Content-Type header:")

				Expect(rr.Body.String()).To(MatchJSON(fmt.Sprintf(`{
					"pagination": {
						"total_results": 0,
						"total_pages": 1,
						"first": {
							"href": "%[1]s/v3/processes/%[2]s/sidecars"
						},
						"last": {
							"href": "%[1]s/v3/processes/%[2]s/sidecars"
						},
						"next": null,
						"previous": null
					},
					"resources": []
				}`, defaultServerURL, processGUID)), "Response body matches response:")
			})
		})

		When("the process isn't accessible to the user", func() {
			BeforeEach(func() {
				processRepo.GetProcessReturns(repositories.ProcessRecord{}, apierrors.NewForbiddenError(nil, repositories.ProcessResourceType))
			})

			It("returns an error", func() {
				expectNotFoundError("Process not found")
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
		const (
			processGUID           = "process-guid"
			spaceGUID             = "space-guid"
			appGUID               = "app-guid"
			createdAt             = "1906-04-18T13:12:00Z"
			updatedAt             = "1906-04-18T13:12:01Z"
			processType           = "web"
			command               = "bundle exec rackup config.ru -p $PORT -o 0.0.0.0"
			memoryInMB      int64 = 256
			diskInMB        int64 = 1024
			healthcheckType       = "port"
			instances             = 5

			baseURL = "https://api.example.org"
		)

		var (
			labels      = map[string]string{}
			annotations = map[string]string{}
		)

		queuePostRequest := func(requestBody string) {
			var err error
			req, err = http.NewRequestWithContext(ctx, "POST", "/v3/processes/"+processGUID+"/actions/scale", strings.NewReader(requestBody))
			Expect(err).NotTo(HaveOccurred())
		}

		BeforeEach(func() {
			processScaler.ScaleProcessReturns(repositories.ProcessRecord{
				GUID:             processGUID,
				SpaceGUID:        spaceGUID,
				AppGUID:          appGUID,
				CreatedAt:        createdAt,
				UpdatedAt:        updatedAt,
				Type:             processType,
				Command:          command,
				DesiredInstances: instances,
				MemoryMB:         memoryInMB,
				DiskQuotaMB:      diskInMB,
				HealthCheck: repositories.HealthCheck{
					Type: healthcheckType,
					Data: repositories.HealthCheckData{},
				},
				Labels:      labels,
				Annotations: annotations,
			}, nil)

			queuePostRequest(fmt.Sprintf(`{
				"instances": %[1]d,
				"memory_in_mb": %[2]d,
				"disk_in_mb": %[3]d
			}`, instances, memoryInMB, diskInMB))
		})

		When("on the happy path and", func() {
			It("passes the authorization.Info to the scale func", func() {
				Expect(processScaler.ScaleProcessCallCount()).To(Equal(1))
				_, actualAuthInfo, _, _ := processScaler.ScaleProcessArgsForCall(0)
				Expect(actualAuthInfo).To(Equal(authInfo))
			})

			When("all scale fields are set", func() {
				It("returns status 200 OK", func() {
					Expect(rr.Code).To(Equal(http.StatusOK), "Matching HTTP response code:")
				})

				It("returns the scaled process", func() {
					contentTypeHeader := rr.Header().Get("Content-Type")
					Expect(contentTypeHeader).To(Equal(jsonHeader), "Matching Content-Type header:")

					Expect(rr.Body.String()).To(MatchJSON(`{
					"guid": "` + processGUID + `",
					"created_at": "` + createdAt + `",
					"updated_at": "` + updatedAt + `",
					"type": "web",
					"command": "bundle exec rackup config.ru -p $PORT -o 0.0.0.0",
					"instances": ` + fmt.Sprint(instances) + `,
					"memory_in_mb": ` + fmt.Sprint(memoryInMB) + `,
					"disk_in_mb": ` + fmt.Sprint(diskInMB) + `,
					"health_check": {
					   "type": "` + healthcheckType + `",
					   "data": {
						  "timeout": null,
						  "invocation_timeout": null
					   }
					},
					"relationships": {
					   "app": {
						  "data": {
							 "guid": "` + appGUID + `"
						  }
					   }
					},
					"metadata": {
					   "labels": {},
					   "annotations": {}
					},
					"links": {
					   "self": {
						  "href": "` + baseURL + `/v3/processes/` + processGUID + `"
					   },
					   "scale": {
						  "href": "` + baseURL + `/v3/processes/` + processGUID + `/actions/scale",
						  "method": "POST"
					   },
					   "app": {
						  "href": "` + baseURL + `/v3/apps/` + appGUID + `"
					   },
					   "space": {
						  "href": "` + baseURL + `/v3/spaces/` + spaceGUID + `"
					   },
					   "stats": {
						  "href": "` + baseURL + `/v3/processes/` + processGUID + `/stats"
					   }
					}
				 }`))
				})
			})

			When("only some fields are set", func() {
				BeforeEach(func() {
					queuePostRequest(fmt.Sprintf(`{
						"memory_in_mb": %[1]d
					}`, memoryInMB))
				})

				It("invokes the scale function with only a subset of the scale fields", func() {
					Expect(processScaler.ScaleProcessCallCount()).To(Equal(1), "did not call scaleProcess just once")
					_, _, _, invokedProcessScale := processScaler.ScaleProcessArgsForCall(0)
					Expect(invokedProcessScale.Instances).To(BeNil())
					Expect(invokedProcessScale.DiskMB).To(BeNil())
					Expect(*invokedProcessScale.MemoryMB).To(Equal(memoryInMB))
				})

				It("returns status 200 OK", func() {
					Expect(rr.Code).To(Equal(http.StatusOK), "Matching HTTP response code:")
				})

				It("returns the scaled process", func() {
					contentTypeHeader := rr.Header().Get("Content-Type")
					Expect(contentTypeHeader).To(Equal(jsonHeader), "Matching Content-Type header:")

					Expect(rr.Body.String()).To(MatchJSON(`{
					"guid": "` + processGUID + `",
					"created_at": "` + createdAt + `",
					"updated_at": "` + updatedAt + `",
					"type": "web",
					"command": "bundle exec rackup config.ru -p $PORT -o 0.0.0.0",
					"instances": ` + fmt.Sprint(instances) + `,
					"memory_in_mb": ` + fmt.Sprint(memoryInMB) + `,
					"disk_in_mb": ` + fmt.Sprint(diskInMB) + `,
					"health_check": {
					   "type": "` + healthcheckType + `",
					   "data": {
						  "timeout": null,
						  "invocation_timeout": null
					   }
					},
					"relationships": {
					   "app": {
						  "data": {
							 "guid": "` + appGUID + `"
						  }
					   }
					},
					"metadata": {
					   "labels": {},
					   "annotations": {}
					},
					"links": {
					   "self": {
						  "href": "` + baseURL + `/v3/processes/` + processGUID + `"
					   },
					   "scale": {
						  "href": "` + baseURL + `/v3/processes/` + processGUID + `/actions/scale",
						  "method": "POST"
					   },
					   "app": {
						  "href": "` + baseURL + `/v3/apps/` + appGUID + `"
					   },
					   "space": {
						  "href": "` + baseURL + `/v3/spaces/` + spaceGUID + `"
					   },
					   "stats": {
						  "href": "` + baseURL + `/v3/processes/` + processGUID + `/stats"
					   }
					}
				 }`))
				})
			})
		})

		When("on the sad path and", func() {
			When("the request JSON is invalid", func() {
				BeforeEach(func() {
					queuePostRequest(`}`)
				})

				It("returns a status 400 Bad Request ", func() {
					Expect(rr.Code).To(Equal(http.StatusBadRequest), "Matching HTTP response code:")
				})

				It("has the expected error response body", func() {
					contentTypeHeader := rr.Header().Get("Content-Type")
					Expect(contentTypeHeader).To(Equal(jsonHeader), "Matching Content-Type header:")

					Expect(rr.Body.String()).To(MatchJSON(`{
						"errors": [
							{
								"title": "CF-MessageParseError",
								"detail": "Request invalid due to parse error: invalid request body",
								"code": 1001
							}
						]
					}`), "Response body matches response:")
				})
			})

			When("there is some other error fetching the process", func() {
				BeforeEach(func() {
					processScaler.ScaleProcessReturns(repositories.ProcessRecord{}, errors.New("unknown!"))
				})

				It("returns an error", func() {
					expectUnknownError()
				})
			})
		})

		When("validating scale parameters", func() {
			DescribeTable("returns a validation decision",
				func(requestBody string, status int) {
					tableTestRecorder := httptest.NewRecorder()
					queuePostRequest(requestBody)
					routerBuilder.Build().ServeHTTP(tableTestRecorder, req)
					Expect(tableTestRecorder.Code).To(Equal(status))
				},
				Entry("instances is negative", `{"instances":-1}`, http.StatusUnprocessableEntity),
				Entry("memory is not a positive integer", `{"memory_in_mb":0}`, http.StatusUnprocessableEntity),
				Entry("disk is not a positive integer", `{"disk_in_mb":0}`, http.StatusUnprocessableEntity),
				Entry("instances is zero", `{"instances":0}`, http.StatusOK),
				Entry("memory is a positive integer", `{"memory_in_mb":1024}`, http.StatusOK),
				Entry("disk is a positive integer", `{"disk_in_mb":1024}`, http.StatusOK),
			)
		})
	})

	Describe("the GET /v3/processes/<guid>/stats endpoint", func() {
		var (
			process1Time, process2Time string
			process1CPU, process2CPU   float64
			process1Mem, process2Mem   int64
			process1Disk, process2Disk int64
		)
		BeforeEach(func() {
			process1Time = "1906-04-18T13:12:00Z"
			process2Time = "1906-04-18T13:12:00Z"
			process1CPU = 133.47
			process2CPU = 127.58
			process1Mem = 16
			process2Mem = 8
			process1Disk = 50
			process2Disk = 100
			processStats.FetchStatsReturns([]actions.PodStatsRecord{
				{
					Type:  "web",
					Index: 0,
					State: "RUNNING",
					Usage: actions.Usage{
						Time: &process1Time,
						CPU:  &process1CPU,
						Mem:  &process1Mem,
						Disk: &process1Disk,
					},
					MemQuota:  tools.PtrTo(int64(1024)),
					DiskQuota: tools.PtrTo(int64(2048)),
				},
				{
					Type:  "web",
					Index: 1,
					State: "RUNNING",
					Usage: actions.Usage{
						Time: &process2Time,
						CPU:  &process2CPU,
						Mem:  &process2Mem,
						Disk: &process2Disk,
					},
					MemQuota:  tools.PtrTo(int64(1024)),
					DiskQuota: tools.PtrTo(int64(2048)),
				},
			}, nil)

			var err error
			req, err = http.NewRequestWithContext(ctx, "GET", "/v3/processes/"+processGUID+"/stats", nil)
			Expect(err).NotTo(HaveOccurred())
		})

		When("on the happy path", func() {
			It("returns status 200 OK", func() {
				Expect(rr.Code).To(Equal(http.StatusOK), "Matching HTTP reponse code:")
			})

			It("passes the authorization.Info to the fetch process stats func", func() {
				Expect(processStats.FetchStatsCallCount()).To(Equal(1))
				_, actualAuthInfo, _ := processStats.FetchStatsArgsForCall(0)
				Expect(actualAuthInfo).To(Equal(authInfo))
			})

			It("returns a canned response with the processGUID in it", func() {
				contentTypeHeader := rr.Header().Get("Content-Type")
				Expect(contentTypeHeader).To(Equal(jsonHeader), "Matching Content-Type header:")

				Expect(rr.Body.String()).To(MatchJSON(fmt.Sprintf(`{
					"resources": [
						{
							"type": "web",
							"index": 0,
							"state": "RUNNING",
							"host": null,
							"uptime": null,
							"mem_quota": 1024,
							"disk_quota": 2048,
							"fds_quota": null,
							"isolation_segment": null,
							"details": null,
							"instance_ports": [],
							"usage": {
								"time": "%s",
								"cpu": %f,
								"mem": %d,
								"disk": %d
                            }
						},
						{
							"type": "web",
							"index": 1,
							"state": "RUNNING",
							"host": null,
							"uptime": null,
							"mem_quota": 1024,
							"disk_quota": 2048,
							"fds_quota": null,
							"isolation_segment": null,
							"details": null,
							"instance_ports": [],
							"usage": {
								"time": "%s",
								"cpu": %f,
								"mem": %d,
								"disk": %d
                            }
						}
					]
				}`, process1Time, process1CPU, process1Mem, process1Disk, process2Time, process2CPU, process2Mem, process2Disk)), "Response body matches response:")
			})
		})

		When("the process is down", func() {
			BeforeEach(func() {
				processStats.FetchStatsReturns([]actions.PodStatsRecord{
					{
						Type:  "web",
						Index: 0,
						State: "DOWN",
					},
				}, nil)
			})
			It("returns a canned response with the processGUID in it", func() {
				contentTypeHeader := rr.Header().Get("Content-Type")
				Expect(contentTypeHeader).To(Equal(jsonHeader), "Matching Content-Type header:")
				Expect(rr.Body.String()).To(MatchJSON(`{
					"resources": [
						{
							"type": "web",
							"index": 0,
							"state": "DOWN",
							"host": null,
							"uptime": null,
							"mem_quota": null,
							"disk_quota": null,
							"fds_quota": null,
							"isolation_segment": null,
							"details": null,
							"usage": {}
						}
					]
				}`), "Response body matches response:")
			})
		})

		When("fetching stats fails with an unauthorized error", func() {
			BeforeEach(func() {
				processStats.FetchStatsReturns(nil, apierrors.NewForbiddenError(nil, repositories.AppResourceType))
			})

			It("returns a not found error", func() {
				expectNotFoundError("App not found")
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
		const (
			processGUID     = "process-guid"
			spaceGUID       = "space-guid"
			appGUID         = "app-guid"
			createdAt       = "1906-04-18T13:12:00Z"
			updatedAt       = "1906-04-18T13:12:01Z"
			processType     = "web"
			command         = "bundle exec rackup config.ru -p $PORT -o 0.0.0.0"
			memoryInMB      = 256
			diskInMB        = 1024
			healthcheckType = "port"
			instances       = 1

			baseURL = "https://api.example.org"
		)

		var (
			labels      = map[string]string{}
			annotations = map[string]string{}
		)

		BeforeEach(func() {
			processRepo.ListProcessesReturns([]repositories.ProcessRecord{
				{
					GUID:             processGUID,
					SpaceGUID:        spaceGUID,
					AppGUID:          appGUID,
					CreatedAt:        createdAt,
					UpdatedAt:        updatedAt,
					Type:             processType,
					Command:          command,
					DesiredInstances: instances,
					MemoryMB:         memoryInMB,
					DiskQuotaMB:      diskInMB,
					HealthCheck: repositories.HealthCheck{
						Type: healthcheckType,
						Data: repositories.HealthCheckData{},
					},
					Labels:      labels,
					Annotations: annotations,
				},
			}, nil)
		})

		When("on the happy path", func() {
			When("Query Parameters are not provided", func() {
				BeforeEach(func() {
					var err error
					req, err = http.NewRequestWithContext(ctx, "GET", "/v3/processes", nil)
					Expect(err).NotTo(HaveOccurred())
				})
				It("returns status 200 OK", func() {
					Expect(rr.Code).To(Equal(http.StatusOK), "Matching HTTP response code:")
				})

				It("returns Content-Type as JSON in header", func() {
					contentTypeHeader := rr.Header().Get("Content-Type")
					Expect(contentTypeHeader).To(Equal(jsonHeader), "Matching Content-Type header:")
				})
				It("returns the Pagination Data and Process Resources in the response", func() {
					Expect(rr.Body.String()).To(MatchJSON(fmt.Sprintf(`{
				"pagination": {
					"total_results": 1,
					"total_pages": 1,
					"first": {
						"href": "`+baseURL+`/v3/processes"
					},
					"last": {
						"href": "`+baseURL+`/v3/processes"
					},
					"next": null,
					"previous": null
				},
				"resources": [
					{
					"guid": "`+processGUID+`",
					"created_at": "`+createdAt+`",
					"updated_at": "`+updatedAt+`",
					"type": "web",
					"command": "[PRIVATE DATA HIDDEN IN LISTS]",
					"instances": `+fmt.Sprint(instances)+`,
					"memory_in_mb": `+fmt.Sprint(memoryInMB)+`,
					"disk_in_mb": `+fmt.Sprint(diskInMB)+`,
					"health_check": {
					   "type": "`+healthcheckType+`",
					   "data": {
						  "timeout": null,
						  "invocation_timeout": null
					   }
					},
					"relationships": {
					   "app": {
						  "data": {
							 "guid": "`+appGUID+`"
						  }
					   }
					},
					"metadata": {
					   "labels": {},
					   "annotations": {}
					},
					"links": {
					   "self": {
						  "href": "`+baseURL+`/v3/processes/`+processGUID+`"
					   },
					   "scale": {
						  "href": "`+baseURL+`/v3/processes/`+processGUID+`/actions/scale",
						  "method": "POST"
					   },
					   "app": {
						  "href": "`+baseURL+`/v3/apps/`+appGUID+`"
					   },
					   "space": {
						  "href": "`+baseURL+`/v3/spaces/`+spaceGUID+`"
					   },
					   "stats": {
						  "href": "`+baseURL+`/v3/processes/`+processGUID+`/stats"
					   }
					}
				 }
				]
				}`)), "Response body matches response:")
				})
			})

			When("Query Parameters are provided", func() {
				BeforeEach(func() {
					var err error
					req, err = http.NewRequestWithContext(ctx, "GET", "/v3/processes?app_guids=my-app-guid", nil)
					Expect(err).NotTo(HaveOccurred())
				})

				It("returns status 200 OK", func() {
					Expect(rr.Code).To(Equal(http.StatusOK), "Matching HTTP response code:")
				})

				It("invokes process repository with correct args", func() {
					_, _, message := processRepo.ListProcessesArgsForCall(0)
					Expect(message.AppGUIDs).To(HaveLen(1))
					Expect(message.AppGUIDs[0]).To(Equal("my-app-guid"))
				})
			})
		})

		When("invalid query parameters are provided", func() {
			BeforeEach(func() {
				var err error
				req, err = http.NewRequestWithContext(ctx, "GET", "/v3/processes?foo=my-app-guid", nil)
				Expect(err).NotTo(HaveOccurred())
			})
			It("returns an Unknown key error", func() {
				expectUnknownKeyError("The query parameter is invalid: Valid parameters are: 'app_guids'")
			})
		})

		When("listing processes fails", func() {
			BeforeEach(func() {
				processRepo.ListProcessesReturns(nil, errors.New("boom"))
				var err error
				req, err = http.NewRequestWithContext(ctx, "GET", "/v3/processes?app_guids=my-app-guid", nil)
				Expect(err).NotTo(HaveOccurred())
			})

			It("returns an error", func() {
				expectUnknownError()
			})
		})
	})

	Describe("the PATCH /v3/processes/:guid endpoint", func() {
		const (
			processGUID     = "process-guid"
			spaceGUID       = "space-guid"
			appGUID         = "app-guid"
			createdAt       = "1906-04-18T13:12:00Z"
			updatedAt       = "1906-04-18T13:12:01Z"
			processType     = "web"
			command         = "bundle exec rackup config.ru -p $PORT -o 0.0.0.0"
			memoryInMB      = 256
			diskInMB        = 1024
			healthcheckType = "port"
			instances       = 1

			baseURL = "https://api.example.org"
		)

		var (
			labels      = map[string]string{}
			annotations = map[string]string{}
		)

		makePatchRequest := func(processGUID, requestBody string) {
			var err error
			req, err = http.NewRequestWithContext(ctx, "PATCH", "/v3/processes/"+processGUID, strings.NewReader(requestBody))
			Expect(err).NotTo(HaveOccurred())
		}

		validBody := `{
		  "health_check": {
			"data": {
			  "invocation_timeout": 2,
              "timeout": 5,
              "endpoint": "http://myapp.com/health"
			},
			"type": "port"
		  }
		}`

		BeforeEach(func() {
			processRepo.GetProcessReturns(repositories.ProcessRecord{
				GUID:             processGUID,
				SpaceGUID:        spaceGUID,
				AppGUID:          appGUID,
				CreatedAt:        createdAt,
				UpdatedAt:        updatedAt,
				Type:             processType,
				Command:          command,
				DesiredInstances: instances,
				MemoryMB:         memoryInMB,
				DiskQuotaMB:      diskInMB,
				HealthCheck: repositories.HealthCheck{
					Type: healthcheckType,
					Data: repositories.HealthCheckData{},
				},
				Labels:      labels,
				Annotations: annotations,
			}, nil)
		})

		When("the request body is valid", func() {
			BeforeEach(func() {
				processRepo.PatchProcessReturns(repositories.ProcessRecord{
					GUID:             processGUID,
					SpaceGUID:        spaceGUID,
					AppGUID:          appGUID,
					CreatedAt:        createdAt,
					UpdatedAt:        updatedAt,
					Type:             processType,
					Command:          command,
					DesiredInstances: instances,
					MemoryMB:         memoryInMB,
					DiskQuotaMB:      diskInMB,
					HealthCheck: repositories.HealthCheck{
						Type: "http",
						Data: repositories.HealthCheckData{
							HTTPEndpoint:             "http://myapp.com/health",
							InvocationTimeoutSeconds: 2,
							TimeoutSeconds:           5,
						},
					},
					Labels:      labels,
					Annotations: annotations,
				}, nil)
			})

			When("the request patches health check", func() {
				BeforeEach(func() {
					makePatchRequest(processGUID, validBody)
				})
				It("returns status 200 OK", func() {
					Expect(rr.Code).To(Equal(http.StatusOK), "Matching HTTP response code:")
				})

				It("passes the authorization.Info to the process repository", func() {
					Expect(processRepo.PatchProcessCallCount()).To(Equal(1))
					_, actualAuthInfo, _ := processRepo.PatchProcessArgsForCall(0)
					Expect(actualAuthInfo).To(Equal(authInfo))
				})

				It("returns a process", func() {
					contentTypeHeader := rr.Header().Get("Content-Type")
					Expect(contentTypeHeader).To(Equal(jsonHeader), "Matching Content-Type header:")

					Expect(rr.Body.String()).To(MatchJSON(`{
					"guid": "` + processGUID + `",
					"created_at": "` + createdAt + `",
					"updated_at": "` + updatedAt + `",
					"type": "web",
					"command": "bundle exec rackup config.ru -p $PORT -o 0.0.0.0",
					"instances": ` + fmt.Sprint(instances) + `,
					"memory_in_mb": ` + fmt.Sprint(memoryInMB) + `,
					"disk_in_mb": ` + fmt.Sprint(diskInMB) + `,
					"health_check": {
					   "type": "http",
					   "data": {
						  "timeout": 5,
						  "invocation_timeout": 2,
                          "endpoint": "http://myapp.com/health"
					   }
					},
					"relationships": {
					   "app": {
						  "data": {
							 "guid": "` + appGUID + `"
						  }
					   }
					},
					"metadata": {
					   "labels": {},
					   "annotations": {}
					},
					"links": {
					   "self": {
						  "href": "` + baseURL + `/v3/processes/` + processGUID + `"
					   },
					   "scale": {
						  "href": "` + baseURL + `/v3/processes/` + processGUID + `/actions/scale",
						  "method": "POST"
					   },
					   "app": {
						  "href": "` + baseURL + `/v3/apps/` + appGUID + `"
					   },
					   "space": {
						  "href": "` + baseURL + `/v3/spaces/` + spaceGUID + `"
					   },
					   "stats": {
						  "href": "` + baseURL + `/v3/processes/` + processGUID + `/stats"
					   }
					}
				 }`))
				})
			})
			When("the request patches metadata", func() {
				BeforeEach(func() {
					makePatchRequest(processGUID, `{"metadata":{"labels":{"foo":"value1"}}}`)
				})

				It("returns status 200 OK", func() {
					Expect(rr.Code).To(Equal(http.StatusOK), "Matching HTTP response code:")
				})

				It("passes the metadata to the patch method on the repository", func() {
					Expect(processRepo.PatchProcessCallCount()).To(Equal(1))
					_, _, msg := processRepo.PatchProcessArgsForCall(0)
					Expect(msg.MetadataPatch.Labels).To(HaveKey("foo"))
				})
			})
		})

		When("the request body is invalid json", func() {
			BeforeEach(func() {
				makePatchRequest(processGUID, `{`)
			})

			It("return an request malformed error", func() {
				expectBadRequestError()
			})
		})

		When("the request body is invalid with an unknown field", func() {
			BeforeEach(func() {
				makePatchRequest(processGUID, `{
				  "health_check": {
					"endpoint": "my-endpoint"
				  }
				}`)
			})

			It("return an request malformed error", func() {
				expectUnprocessableEntityError("invalid request body: json: unknown field \"endpoint\"")
			})
		})

		When("user is not allowed to get a process", func() {
			BeforeEach(func() {
				processRepo.GetProcessReturns(repositories.ProcessRecord{}, apierrors.NewForbiddenError(errors.New("nope"), repositories.ProcessResourceType))
				makePatchRequest(processGUID, validBody)
			})

			It("returns a not found error", func() {
				expectNotFoundError("Process not found")
			})
		})

		When("getting the process fails a process", func() {
			BeforeEach(func() {
				processRepo.GetProcessReturns(repositories.ProcessRecord{}, errors.New("boom"))
				makePatchRequest(processGUID, validBody)
			})

			It("returns an error", func() {
				expectUnknownError()
			})
		})

		When("patching the process fails a process", func() {
			BeforeEach(func() {
				processRepo.PatchProcessReturns(repositories.ProcessRecord{}, errors.New("boom"))
				makePatchRequest(processGUID, validBody)
			})

			It("returns an error", func() {
				expectUnknownError()
			})
		})
	})
})
