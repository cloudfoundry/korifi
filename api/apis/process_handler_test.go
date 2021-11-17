package apis_test

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"

	. "code.cloudfoundry.org/cf-k8s-controllers/api/apis"
	"code.cloudfoundry.org/cf-k8s-controllers/api/apis/fake"
	"code.cloudfoundry.org/cf-k8s-controllers/api/repositories"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"k8s.io/client-go/rest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

var _ = Describe("ProcessHandler", func() {
	const (
		processGUID = "test-process-guid"
	)

	var (
		processRepo      *fake.CFProcessRepository
		scaleProcessFunc *fake.ScaleProcess
		clientBuilder    *fake.ClientBuilder
	)

	BeforeEach(func() {
		processRepo = new(fake.CFProcessRepository)
		scaleProcessFunc = new(fake.ScaleProcess)
		clientBuilder = new(fake.ClientBuilder)

		apiHandler := NewProcessHandler(
			logf.Log.WithName(testAppHandlerLoggerName),
			*serverURL,
			processRepo,
			scaleProcessFunc.Spy,
			clientBuilder.Spy,
			&rest.Config{},
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
			processRepo.FetchProcessReturns(repositories.ProcessRecord{
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
			req, err = http.NewRequest("GET", "/v3/processes/"+processGUID, nil)
			Expect(err).NotTo(HaveOccurred())
		})

		When("on the happy path", func() {
			It("returns status 200 OK", func() {
				Expect(rr.Code).To(Equal(http.StatusOK), "Matching HTTP response code:")
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
					processRepo.FetchProcessReturns(repositories.ProcessRecord{}, repositories.NotFoundError{})
				})

				It("returns an error", func() {
					expectNotFoundError("Process not found")
				})
			})

			When("there is some other error fetching the process", func() {
				BeforeEach(func() {
					processRepo.FetchProcessReturns(repositories.ProcessRecord{}, errors.New("unknown!"))
				})

				It("returns an error", func() {
					expectUnknownError()
				})
			})
		})
	})

	Describe("the GET /v3/processes/:guid/sidecars endpoint", func() {
		BeforeEach(func() {
			processRepo.FetchProcessReturns(repositories.ProcessRecord{}, nil)

			var err error
			req, err = http.NewRequest("GET", "/v3/processes/"+processGUID+"/sidecars", nil)
			Expect(err).NotTo(HaveOccurred())
		})

		When("on the happy path", func() {
			It("returns status 200 OK", func() {
				Expect(rr.Code).To(Equal(http.StatusOK), "Matching HTTP response code:")
			})

			It("returns a canned response with the processGUID in it", func() {
				contentTypeHeader := rr.Header().Get("Content-Type")
				Expect(contentTypeHeader).To(Equal(jsonHeader), "Matching Content-Type header:")

				Expect(rr.Body.String()).To(MatchJSON(fmt.Sprintf(`{
					"pagination": {
						"total_results": 0,
						"total_pages": 1,
						"first": {
							"href": "%[1]s/v3/processes/%[2]s/sidecars?page=1"
						},
						"last": {
							"href": "%[1]s/v3/processes/%[2]s/sidecars?page=1"
						},
						"next": null,
						"previous": null
					},
					"resources": []
				}`, defaultServerURL, processGUID)), "Response body matches response:")
			})
		})

		When("on the sad path and", func() {
			When("the process doesn't exist", func() {
				BeforeEach(func() {
					processRepo.FetchProcessReturns(repositories.ProcessRecord{}, repositories.NotFoundError{})
				})

				It("returns an error", func() {
					expectNotFoundError("Process not found")
				})
			})

			When("there is some other error fetching the process", func() {
				BeforeEach(func() {
					processRepo.FetchProcessReturns(repositories.ProcessRecord{}, errors.New("unknown!"))
				})

				It("returns an error", func() {
					expectUnknownError()
				})
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
			req, err = http.NewRequest("POST", "/v3/processes/"+processGUID+"/actions/scale", strings.NewReader(requestBody))
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
					scaleProcessFunc.Returns(repositories.ProcessRecord{}, repositories.NotFoundError{})
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
		})

		When("the validating scale parameters", func() {
			DescribeTable("returns validation",
				func(requestBody string, status int) {
					var tableTestRecorder *httptest.ResponseRecorder = httptest.NewRecorder()
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
})
