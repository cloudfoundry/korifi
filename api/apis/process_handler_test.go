package apis_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"

	. "code.cloudfoundry.org/cf-k8s-controllers/api/apis"
	"code.cloudfoundry.org/cf-k8s-controllers/api/apis/fake"
	"code.cloudfoundry.org/cf-k8s-controllers/api/repositories"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

var _ = Describe("ProcessHandler", func() {
	const (
		processGUID = "test-process-guid"
	)

	var (
		processRepo       *fake.CFProcessRepository
		fetchProcessStats *fake.FetchProcessStats
		scaleProcessFunc  *fake.ScaleProcess
		req               *http.Request
	)

	BeforeEach(func() {
		processRepo = new(fake.CFProcessRepository)
		fetchProcessStats = new(fake.FetchProcessStats)
		scaleProcessFunc = new(fake.ScaleProcess)
		decoderValidator, err := NewDefaultDecoderValidator()
		Expect(err).NotTo(HaveOccurred())

		apiHandler := NewProcessHandler(
			logf.Log.WithName(testAppHandlerLoggerName),
			*serverURL,
			processRepo,
			fetchProcessStats.Spy,
			scaleProcessFunc.Spy,
			decoderValidator,
		)
		apiHandler.RegisterRoutes(router)
	})

	JustBeforeEach(func() {
		router.ServeHTTP(rr, req)
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
			When("the process doesn't exist", func() {
				BeforeEach(func() {
					processRepo.GetProcessReturns(repositories.ProcessRecord{}, repositories.NewNotFoundError(repositories.ProcessResourceType, nil))
				})

				It("returns a not-found error", func() {
					expectNotFoundError("Process not found")
				})
			})
			When("the user lacks access", func() {
				BeforeEach(func() {
					processRepo.GetProcessReturns(repositories.ProcessRecord{}, repositories.NewForbiddenError(repositories.ProcessResourceType, errors.New("access denied or something")))
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

			When("the authorization.Info is not set in the request context", func() {
				BeforeEach(func() {
					var err error
					req, err = http.NewRequest("GET", "/v3/processes/"+processGUID, nil)
					Expect(err).NotTo(HaveOccurred())
				})

				It("returns an unknown error", func() {
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

		When("the process doesn't exist", func() {
			BeforeEach(func() {
				processRepo.GetProcessReturns(repositories.ProcessRecord{}, repositories.NewNotFoundError(repositories.ProcessResourceType, nil))
			})

			It("returns an error", func() {
				expectNotFoundError("Process not found")
			})
		})

		When("the process isn't accessible to the user", func() {
			BeforeEach(func() {
				processRepo.GetProcessReturns(repositories.ProcessRecord{}, repositories.NewForbiddenError(repositories.ProcessResourceType, nil))
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

		When("the authorization.Info is not set in the request context", func() {
			BeforeEach(func() {
				var err error
				req, err = http.NewRequest("GET", "/v3/processes/"+processGUID, nil)
				Expect(err).NotTo(HaveOccurred())
			})

			It("returns an unknown error", func() {
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
			scaleProcessFunc.Returns(repositories.ProcessRecord{
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
				Expect(scaleProcessFunc.CallCount()).To(Equal(1))
				_, actualAuthInfo, _, _ := scaleProcessFunc.ArgsForCall(0)
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
					Expect(scaleProcessFunc.CallCount()).To(Equal(1), "did not call scaleProcess just once")
					_, _, _, invokedProcessScale := scaleProcessFunc.ArgsForCall(0)
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

			When("the process doesn't exist", func() {
				BeforeEach(func() {
					scaleProcessFunc.Returns(repositories.ProcessRecord{}, repositories.NewNotFoundError(repositories.ProcessResourceType, nil))
				})

				It("returns an error", func() {
					expectNotFoundError("Process not found")
				})
			})

			When("there is some other error fetching the process", func() {
				BeforeEach(func() {
					scaleProcessFunc.Returns(repositories.ProcessRecord{}, errors.New("unknown!"))
				})

				It("returns an error", func() {
					expectUnknownError()
				})
			})

			When("authorization.Info is not set in the request context", func() {
				BeforeEach(func() {
					ctx = context.Background()

					queuePostRequest(fmt.Sprintf(`{
                        "instances": %[1]d,
                        "memory_in_mb": %[2]d,
                        "disk_in_mb": %[3]d
                    }`, instances, memoryInMB, diskInMB))
				})

				It("returns an unknown error", func() {
					expectUnknownError()
				})
			})
		})

		When("validating scale parameters", func() {
			DescribeTable("returns a validation decision",
				func(requestBody string, status int) {
					tableTestRecorder := httptest.NewRecorder()
					queuePostRequest(requestBody)
					router.ServeHTTP(tableTestRecorder, req)
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
		BeforeEach(func() {
			fetchProcessStats.Returns([]repositories.PodStatsRecord{
				{
					Type:  "web",
					Index: 0,
					State: "RUNNING",
				},
				{
					Type:  "web",
					Index: 1,
					State: "RUNNING",
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
				Expect(fetchProcessStats.CallCount()).To(Equal(1))
				_, actualAuthInfo, _ := fetchProcessStats.ArgsForCall(0)
				Expect(actualAuthInfo).To(Equal(authInfo))
			})

			It("returns a canned response with the processGUID in it", func() {
				contentTypeHeader := rr.Header().Get("Content-Type")
				Expect(contentTypeHeader).To(Equal(jsonHeader), "Matching Content-Type header:")

				Expect(rr.Body.String()).To(MatchJSON(`{
					"resources": [
						{
							"type": "web",
							"index": 0,
							"state": "RUNNING",
							"host": null,
							"uptime": null,
							"mem_quota": null,
							"disk_quota": null,
							"fds_quota": null,
							"isolation_segment": null,
							"details": null,
							"instance_ports": []
						},
						{
							"type": "web",
							"index": 1,
							"state": "RUNNING",
							"host": null,
							"uptime": null,
							"mem_quota": null,
							"disk_quota": null,
							"fds_quota": null,
							"isolation_segment": null,
							"details": null,
							"instance_ports": []
						}
					]
				}`), "Response body matches response:")
			})
		})

		When("the process is down", func() {
			BeforeEach(func() {
				fetchProcessStats.Returns([]repositories.PodStatsRecord{
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
							"details": null
						}
					]
				}`), "Response body matches response:")
			})
		})

		When("the process is not found", func() {
			BeforeEach(func() {
				fetchProcessStats.Returns(nil, repositories.NewNotFoundError(repositories.ProcessResourceType, nil))
			})
			It("an error", func() {
				expectNotFoundError("Process not found")
			})
		})

		When("the app is not found", func() {
			BeforeEach(func() {
				fetchProcessStats.Returns(nil, repositories.NewNotFoundError(repositories.AppResourceType, nil))
			})
			It("an error", func() {
				expectNotFoundError("App not found")
			})
		})

		When("authorization.Info is not set in the request context", func() {
			BeforeEach(func() {
				var err error
				req, err = http.NewRequest("GET", "/v3/processes/"+processGUID+"/stats", nil)
				Expect(err).NotTo(HaveOccurred())
			})

			It("returns an unknown error", func() {
				expectUnknownError()
			})
		})

		When("the app is not authorized", func() {
			BeforeEach(func() {
				fetchProcessStats.Returns(nil, repositories.NewForbiddenError(repositories.AppResourceType, nil))
			})

			It("returns an error", func() {
				expectNotFoundError("App not found")
			})
		})

		When("the process is not authorized", func() {
			BeforeEach(func() {
				fetchProcessStats.Returns(nil, repositories.NewForbiddenError(repositories.ProcessResourceType, nil))
			})

			It("returns an error", func() {
				expectNotFoundError("Process not found")
			})
		})

		When("the process stats are not authorized", func() {
			BeforeEach(func() {
				fetchProcessStats.Returns(nil, repositories.NewForbiddenError(repositories.ProcessStatsResourceType, nil))
			})

			It("returns an error", func() {
				expectNotFoundError("Process Stats not found")
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
					Expect(message.AppGUID).To(HaveLen(1))
					Expect(message.AppGUID[0]).To(Equal("my-app-guid"))
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
				processRepo.GetProcessReturns(repositories.ProcessRecord{}, repositories.NewForbiddenError(repositories.ProcessResourceType, errors.New("nope")))
				makePatchRequest(processGUID, validBody)
			})

			It("returns an unauthorised error", func() {
				expectNotFoundError("Process not found")
			})
		})
	})
})
