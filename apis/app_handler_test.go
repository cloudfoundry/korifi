package apis_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"

	. "code.cloudfoundry.org/cf-k8s-api/apis"
	"code.cloudfoundry.org/cf-k8s-api/apis/fake"
	"code.cloudfoundry.org/cf-k8s-api/repositories"

	"github.com/gorilla/mux"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/rest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	appGUID                  = "test-app-guid"
	appName                  = "test-app"
	spaceGUID                = "test-space-guid"
	testAppHandlerLoggerName = "TestAppHandler"
)

var _ = Describe("AppHandler", func() {
	var (
		rr            *httptest.ResponseRecorder
		req           *http.Request
		appRepo       *fake.CFAppRepository
		dropletRepo   *fake.CFDropletRepository
		processRepo   *fake.CFProcessRepository
		clientBuilder *fake.ClientBuilder
		router        *mux.Router
	)

	getRR := func() *httptest.ResponseRecorder { return rr }

	BeforeEach(func() {
		appRepo = new(fake.CFAppRepository)
		dropletRepo = new(fake.CFDropletRepository)
		processRepo = new(fake.CFProcessRepository)
		clientBuilder = new(fake.ClientBuilder)

		rr = httptest.NewRecorder()
		router = mux.NewRouter()
		serverURL, err := url.Parse(defaultServerURL)
		Expect(err).NotTo(HaveOccurred())
		apiHandler := NewAppHandler(
			logf.Log.WithName(testAppHandlerLoggerName),
			*serverURL,
			appRepo,
			dropletRepo,
			processRepo,
			clientBuilder.Spy,
			&rest.Config{},
		)
		apiHandler.RegisterRoutes(router)
	})

	JustBeforeEach(func() {
		router.ServeHTTP(rr, req)
	})

	Describe("the GET /v3/apps/:guid endpoint", func() {
		BeforeEach(func() {
			appRepo.FetchAppReturns(repositories.AppRecord{
				GUID:      appGUID,
				Name:      "test-app",
				SpaceGUID: spaceGUID,
				State:     "STOPPED",
				Lifecycle: repositories.Lifecycle{
					Type: "buildpack",
					Data: repositories.LifecycleData{
						Buildpacks: []string{},
						Stack:      "",
					},
				},
			}, nil)

			var err error
			req, err = http.NewRequest("GET", "/v3/apps/"+appGUID, nil)
			Expect(err).NotTo(HaveOccurred())
		})

		When("on the happy path", func() {
			It("returns status 200 OK", func() {
				Expect(rr.Code).To(Equal(http.StatusOK), "Matching HTTP response code:")
			})

			It("returns the App in the response", func() {
				contentTypeHeader := rr.Header().Get("Content-Type")
				Expect(contentTypeHeader).To(Equal(jsonHeader), "Matching Content-Type header:")

				Expect(rr.Body.String()).To(MatchJSON(fmt.Sprintf(`{
                    "guid": "%[2]s",
                    "created_at": "",
                    "updated_at": "",
                    "name": "test-app",
                    "state": "STOPPED",
                    "lifecycle": {
                      "type": "buildpack",
                      "data": {
                        "buildpacks": [],
                        "stack": ""
                      }
                    },
                    "relationships": {
                      "space": {
                        "data": {
                          "guid": "%[3]s"
                        }
                      }
                    },
                    "metadata": {
                      "labels": {},
                      "annotations": {}
                    },
                    "links": {
                      "self": {
                        "href": "https://api.example.org/v3/apps/%[2]s"
                      },
                      "environment_variables": {
                        "href": "https://api.example.org/v3/apps/%[2]s/environment_variables"
                      },
                      "space": {
                        "href": "https://api.example.org/v3/spaces/%[3]s"
                      },
                      "processes": {
                        "href": "https://api.example.org/v3/apps/%[2]s/processes"
                      },
                      "packages": {
                        "href": "https://api.example.org/v3/apps/%[2]s/packages"
                      },
                      "current_droplet": {
                        "href": "https://api.example.org/v3/apps/%[2]s/droplets/current"
                      },
                      "droplets": {
                        "href": "https://api.example.org/v3/apps/%[2]s/droplets"
                      },
                      "tasks": {
                        "href": "https://api.example.org/v3/apps/%[2]s/tasks"
                      },
                      "start": {
                        "href": "https://api.example.org/v3/apps/%[2]s/actions/start",
                        "method": "POST"
                      },
                      "stop": {
                        "href": "https://api.example.org/v3/apps/%[2]s/actions/stop",
                        "method": "POST"
                      },
                      "revisions": {
                        "href": "https://api.example.org/v3/apps/%[2]s/revisions"
                      },
                      "deployed_revisions": {
                        "href": "https://api.example.org/v3/apps/%[2]s/revisions/deployed"
                      },
                      "features": {
                        "href": "https://api.example.org/v3/apps/%[2]s/features"
                      }
                    }
                }`, defaultServerURL, appGUID, spaceGUID)), "Response body matches response:")
			})
		})
		When("the app cannot be found", func() {
			BeforeEach(func() {
				appRepo.FetchAppReturns(repositories.AppRecord{}, repositories.NotFoundError{})
			})

			// TODO: should we return code 100004 instead?
			itRespondsWithNotFound("App not found", getRR)
		})

		When("there is some other error fetching the app", func() {
			BeforeEach(func() {
				appRepo.FetchAppReturns(repositories.AppRecord{}, errors.New("unknown!"))
			})

			itRespondsWithUnknownError(getRR)
		})
	})

	Describe("the POST /v3/apps endpoint", func() {
		const (
			testAppName = "test-app"
		)

		queuePostRequest := func(requestBody string) {
			var err error
			req, err = http.NewRequest("POST", "/v3/apps", strings.NewReader(requestBody))
			Expect(err).NotTo(HaveOccurred())
		}

		When("the request body is invalid json", func() {
			BeforeEach(func() {
				queuePostRequest(`{`)
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

		When("the request body does not validate", func() {
			BeforeEach(func() {
				queuePostRequest(`{"description" : "Invalid Request"}`)
			})

			itRespondsWithUnprocessableEntity(`invalid request body: json: unknown field "description"`, getRR)
		})

		When("the request body is invalid with invalid app name", func() {
			BeforeEach(func() {
				queuePostRequest(`{
					"name": 12345,
					"relationships": { "space": { "data": { "guid": "2f35885d-0c9d-4423-83ad-fd05066f8576" } } }
				}`)
			})

			itRespondsWithUnprocessableEntity("Name must be a string", getRR)
		})

		When("the request body is invalid with invalid environment variable object", func() {
			BeforeEach(func() {
				queuePostRequest(`{
					"name": "my_app",
					"environment_variables": [],
					"relationships": { "space": { "data": { "guid": "2f35885d-0c9d-4423-83ad-fd05066f8576" } } }
				}`)
			})

			itRespondsWithUnprocessableEntity("Environment_variables must be a map[string]string", getRR)
		})

		When("the request body is invalid with missing required name field", func() {
			BeforeEach(func() {
				queuePostRequest(`{
					"relationships": { "space": { "data": { "guid": "0c78dd5d-c723-4f2e-b168-df3c3e1d0806" } } }
				}`)
			})

			itRespondsWithUnprocessableEntity("Name is a required field", getRR)
		})

		When("the request body is invalid with missing data within lifecycle", func() {
			BeforeEach(func() {
				queuePostRequest(`{
					"name": "test-app",
					"lifecycle":{},
					"relationships": { "space": { "data": { "guid": "0c78dd5d-c723-4f2e-b168-df3c3e1d0806" } } }
				}`)
			})

			It("returns a status 422 Unprocessable Entity", func() {
				Expect(rr.Code).To(Equal(http.StatusUnprocessableEntity), "Matching HTTP response code:")
			})

			It("has the expected error response body", func() {
				contentTypeHeader := rr.Header().Get("Content-Type")
				Expect(contentTypeHeader).To(Equal(jsonHeader), "Matching Content-Type header:")

				decoder := json.NewDecoder(rr.Body)
				decoder.DisallowUnknownFields()

				body := struct {
					Errors []struct {
						Title  string `json:"title"`
						Code   int    `json:"code"`
						Detail string `json:"detail"`
					} `json:"errors"`
				}{}
				Expect(decoder.Decode(&body)).To(Succeed())

				Expect(body.Errors).To(HaveLen(1))
				Expect(body.Errors[0].Title).To(Equal("CF-UnprocessableEntity"))
				Expect(body.Errors[0].Code).To(Equal(10008))
				Expect(body.Errors[0].Detail).NotTo(BeEmpty())
				subDetails := strings.Split(body.Errors[0].Detail, ",")
				Expect(subDetails).To(ConsistOf(
					"Type is a required field",
					"Buildpacks is a required field",
					"Stack is a required field",
				))
			})
		})

		When("the space does not exist", func() {
			BeforeEach(func() {
				appRepo.FetchNamespaceReturns(
					repositories.SpaceRecord{},
					repositories.PermissionDeniedOrNotFoundError{Err: errors.New("not found")},
				)

				requestBody := initializeCreateAppRequestBody(testAppName, "no-such-guid", nil, nil, nil)
				queuePostRequest(requestBody)
			})

			itRespondsWithUnprocessableEntity("Invalid space. Ensure that the space exists and you have access to it.", getRR)
		})

		When("the app already exists, but AppCreate returns false due to validating webhook rejection", func() {
			BeforeEach(func() {
				controllerError := new(k8serrors.StatusError)
				controllerError.ErrStatus.Reason = `{"code":1,"message":"CFApp with the same spec.name exists"}`
				appRepo.CreateAppReturns(repositories.AppRecord{}, controllerError)

				requestBody := initializeCreateAppRequestBody(testAppName, spaceGUID, nil, nil, nil)
				queuePostRequest(requestBody)
			})

			It("returns a status 422 Unprocessable Entity", func() {
				Expect(rr.Code).To(Equal(http.StatusUnprocessableEntity), "Matching HTTP response code:")
			})

			It("returns a CF API formatted Error response", func() {
				contentTypeHeader := rr.Header().Get("Content-Type")
				Expect(contentTypeHeader).To(Equal(jsonHeader), "Matching Content-Type header:")

				Expect(rr.Body.String()).To(MatchJSON(`{
				"errors": [
					{
						"title": "CF-UniquenessError",
						"detail": "App with the name 'test-app' already exists.",
						"code": 10016
					}
				]
			}`), "Response body matches response:")
			})
		})

		When("the app already exists, but CreateApp returns false due to a non webhook k8s error", func() {
			BeforeEach(func() {
				controllerError := new(k8serrors.StatusError)
				controllerError.ErrStatus.Reason = "different k8s api error"
				appRepo.CreateAppReturns(repositories.AppRecord{}, controllerError)

				requestBody := initializeCreateAppRequestBody(testAppName, spaceGUID, nil, nil, nil)
				queuePostRequest(requestBody)
			})

			itRespondsWithUnknownError(getRR)
		})

		When("the namespace exists and app does not exist and", func() {
			When("a plain POST test app request is sent without env vars or metadata", func() {
				BeforeEach(func() {
					appRepo.CreateAppReturns(repositories.AppRecord{
						GUID:      appGUID,
						Name:      testAppName,
						SpaceGUID: spaceGUID,
						State:     repositories.DesiredState("STOPPED"),
						Lifecycle: repositories.Lifecycle{
							Type: "buildpack",
							Data: repositories.LifecycleData{
								Buildpacks: []string{},
								Stack:      "",
							},
						},
					}, nil)

					requestBody := initializeCreateAppRequestBody(testAppName, spaceGUID, nil, nil, nil)
					queuePostRequest(requestBody)
				})

				It("should invoke repo CreateApp with a random GUID", func() {
					Expect(appRepo.CreateAppCallCount()).To(Equal(1), "Repo CreateApp count was not invoked 1 time")
					_, _, createAppRecord := appRepo.CreateAppArgsForCall(0)
					Expect(createAppRecord.GUID).To(MatchRegexp("^[-0-9a-f]{36}$"), "CreateApp record GUID was not a 36 character guid")
				})

				It("should not invoke repo CreateAppEnvironmentVariables when no environment variables are provided", func() {
					Expect(appRepo.CreateAppEnvironmentVariablesCallCount()).To(BeZero(), "Repo CreateAppEnvironmentVariables was invoked even though no environment vars were provided")
				})

				It("return status 200 OK", func() {
					Expect(rr.Code).To(Equal(http.StatusOK), "Matching HTTP response code:")
				})

				It("returns Content-Type as JSON in header", func() {
					contentTypeHeader := rr.Header().Get("Content-Type")
					Expect(contentTypeHeader).To(Equal(jsonHeader), "Matching Content-Type header:")
				})

				It("returns the \"created app\"(the mock response record) in the response", func() {
					Expect(rr.Body.String()).To(MatchJSON(fmt.Sprintf(`{
					"guid": "%[2]s",
					"created_at": "",
					"updated_at": "",
					"name": "test-app",
					"state": "STOPPED",
					"lifecycle": {
					  "type": "buildpack",
					  "data": {
						"buildpacks": [],
						"stack": ""
					  }
					},
					"relationships": {
					  "space": {
						"data": {
						  "guid": "%[3]s"
						}
					  }
					},
					"metadata": {
					  "labels": {},
					  "annotations": {}
					},
					"links": {
					  "self": {
						"href": "%[1]s/v3/apps/%[2]s"
					  },
					  "environment_variables": {
						"href": "%[1]s/v3/apps/%[2]s/environment_variables"
					  },
					  "space": {
						"href": "%[1]s/v3/spaces/%[3]s"
					  },
					  "processes": {
						"href": "%[1]s/v3/apps/%[2]s/processes"
					  },
					  "packages": {
						"href": "%[1]s/v3/apps/%[2]s/packages"
					  },
					  "current_droplet": {
						"href": "%[1]s/v3/apps/%[2]s/droplets/current"
					  },
					  "droplets": {
						"href": "%[1]s/v3/apps/%[2]s/droplets"
					  },
					  "tasks": {
						"href": "%[1]s/v3/apps/%[2]s/tasks"
					  },
					  "start": {
						"href": "%[1]s/v3/apps/%[2]s/actions/start",
						"method": "POST"
					  },
					  "stop": {
						"href": "%[1]s/v3/apps/%[2]s/actions/stop",
						"method": "POST"
					  },
					  "revisions": {
						"href": "%[1]s/v3/apps/%[2]s/revisions"
					  },
					  "deployed_revisions": {
						"href": "%[1]s/v3/apps/%[2]s/revisions/deployed"
					  },
					  "features": {
						"href": "%[1]s/v3/apps/%[2]s/features"
					  }
					}
				}`, defaultServerURL, appGUID, spaceGUID)), "Response body matches response:")
				})
			})

			When("a POST test app request is sent with env vars and", func() {
				var (
					testEnvironmentVariables map[string]string
					requestBody              string
				)

				BeforeEach(func() {
					testEnvironmentVariables = map[string]string{"foo": "foo", "bar": "bar"}

					requestBody = initializeCreateAppRequestBody(testAppName, spaceGUID, testEnvironmentVariables, nil, nil)
				})

				When("the env var repository is working and will not return an error", func() {
					const createEnvVarsResponseName = "testAppGUID-env"

					BeforeEach(func() {
						appRepo.CreateAppEnvironmentVariablesReturns(repositories.AppEnvVarsRecord{
							Name: createEnvVarsResponseName,
						}, nil)

						queuePostRequest(requestBody)
					})

					It("should call Repo CreateAppEnvironmentVariables with the space and environment vars", func() {
						Expect(appRepo.CreateAppEnvironmentVariablesCallCount()).To(Equal(1), "Repo CreateAppEnvironmentVariables count was not invoked 1 time")
						_, _, createAppEnvVarsRecord := appRepo.CreateAppEnvironmentVariablesArgsForCall(0)
						Expect(createAppEnvVarsRecord.EnvironmentVariables).To(Equal(testEnvironmentVariables))
						Expect(createAppEnvVarsRecord.SpaceGUID).To(Equal(spaceGUID))
					})

					It("should call Repo CreateApp and provide the name of the created env Secret", func() {
						Expect(appRepo.CreateAppCallCount()).To(Equal(1), "Repo CreateApp count was not invoked 1 time")
						_, _, createAppRecord := appRepo.CreateAppArgsForCall(0)
						Expect(createAppRecord.EnvSecretName).To(Equal(createEnvVarsResponseName))
					})
				})

				When("there will be a repository error with creating the env vars", func() {
					BeforeEach(func() {
						appRepo.CreateAppEnvironmentVariablesReturns(repositories.AppEnvVarsRecord{}, errors.New("intentional error"))

						queuePostRequest(requestBody)
					})

					itRespondsWithUnknownError(getRR)
				})
			})

			When("a POST test app request is sent with metadata labels", func() {
				var testLabels map[string]string

				BeforeEach(func() {
					testLabels = map[string]string{"foo": "foo", "bar": "bar"}

					requestBody := initializeCreateAppRequestBody(testAppName, spaceGUID, nil, testLabels, nil)
					queuePostRequest(requestBody)
				})

				It("should pass along the labels to CreateApp", func() {
					Expect(appRepo.CreateAppCallCount()).To(Equal(1), "Repo CreateApp count was not invoked 1 time")
					_, _, createAppRecord := appRepo.CreateAppArgsForCall(0)
					Expect(createAppRecord.Labels).To(Equal(testLabels))
				})
			})

			When("a POST test app request is sent with metadata annotations", func() {
				var testAnnotations map[string]string

				BeforeEach(func() {
					testAnnotations = map[string]string{"foo": "foo", "bar": "bar"}
					requestBody := initializeCreateAppRequestBody(testAppName, spaceGUID, nil, nil, testAnnotations)
					queuePostRequest(requestBody)
				})

				It("should pass along the annotations to CreateApp", func() {
					Expect(appRepo.CreateAppCallCount()).To(Equal(1), "Repo CreateApp count was not invoked 1 time")
					_, _, createAppRecord := appRepo.CreateAppArgsForCall(0)
					Expect(createAppRecord.Annotations).To(Equal(testAnnotations))
				})
			})
		})
	})

	Describe("the GET /v3/apps endpoint", func() {
		BeforeEach(func() {
			appRepo.FetchAppListReturns([]repositories.AppRecord{
				{
					GUID:      "first-test-app-guid",
					Name:      "first-test-app",
					SpaceGUID: "test-space-guid",
					State:     "STOPPED",
					Lifecycle: repositories.Lifecycle{
						Type: "buildpack",
						Data: repositories.LifecycleData{
							Buildpacks: []string{},
							Stack:      "",
						},
					},
				},
				{
					GUID:      "second-test-app-guid",
					Name:      "second-test-app",
					SpaceGUID: "test-space-guid",
					State:     "STOPPED",
					Lifecycle: repositories.Lifecycle{
						Type: "buildpack",
						Data: repositories.LifecycleData{
							Buildpacks: []string{},
							Stack:      "",
						},
					},
				},
			}, nil)

			var err error
			req, err = http.NewRequest("GET", "/v3/apps", nil)
			Expect(err).NotTo(HaveOccurred())
		})

		When("on the happy path", func() {
			It("returns status 200 OK", func() {
				Expect(rr.Code).Should(Equal(http.StatusOK), "Matching HTTP response code:")
			})

			It("returns Content-Type as JSON in header", func() {
				contentTypeHeader := rr.Header().Get("Content-Type")
				Expect(contentTypeHeader).Should(Equal(jsonHeader), "Matching Content-Type header:")
			})

			It("returns the Pagination Data and App Resources in the response", func() {
				Expect(rr.Body.String()).Should(MatchJSON(fmt.Sprintf(`{
				"pagination": {
				  "total_results": 2,
				  "total_pages": 1,
				  "first": {
					"href": "%[1]s/v3/apps?page=1"
				  },
				  "last": {
					"href": "%[1]s/v3/apps?page=1"
				  },
				  "next": null,
				  "previous": null
				},
				"resources": [
					{
						"guid": "first-test-app-guid",
						"created_at": "",
						"updated_at": "",
						"name": "first-test-app",
						"state": "STOPPED",
						"lifecycle": {
						  "type": "buildpack",
						  "data": {
							"buildpacks": [],
							"stack": ""
						  }
						},
						"relationships": {
						  "space": {
							"data": {
							  "guid": "test-space-guid"
							}
						  }
						},
						"metadata": {
						  "labels": {},
						  "annotations": {}
						},
						"links": {
						  "self": {
							"href": "%[1]s/v3/apps/first-test-app-guid"
						  },
						  "environment_variables": {
							"href": "%[1]s/v3/apps/first-test-app-guid/environment_variables"
						  },
						  "space": {
							"href": "%[1]s/v3/spaces/test-space-guid"
						  },
						  "processes": {
							"href": "%[1]s/v3/apps/first-test-app-guid/processes"
						  },
						  "packages": {
							"href": "%[1]s/v3/apps/first-test-app-guid/packages"
						  },
						  "current_droplet": {
							"href": "%[1]s/v3/apps/first-test-app-guid/droplets/current"
						  },
						  "droplets": {
							"href": "%[1]s/v3/apps/first-test-app-guid/droplets"
						  },
						  "tasks": {
							"href": "%[1]s/v3/apps/first-test-app-guid/tasks"
						  },
						  "start": {
							"href": "%[1]s/v3/apps/first-test-app-guid/actions/start",
							"method": "POST"
						  },
						  "stop": {
							"href": "%[1]s/v3/apps/first-test-app-guid/actions/stop",
							"method": "POST"
						  },
						  "revisions": {
							"href": "%[1]s/v3/apps/first-test-app-guid/revisions"
						  },
						  "deployed_revisions": {
							"href": "%[1]s/v3/apps/first-test-app-guid/revisions/deployed"
						  },
						  "features": {
							"href": "%[1]s/v3/apps/first-test-app-guid/features"
						  }
						}
					},
					{
						"guid": "second-test-app-guid",
						"created_at": "",
						"updated_at": "",
						"name": "second-test-app",
						"state": "STOPPED",
						"lifecycle": {
						  "type": "buildpack",
						  "data": {
							"buildpacks": [],
							"stack": ""
						  }
						},
						"relationships": {
						  "space": {
							"data": {
							  "guid": "test-space-guid"
							}
						  }
						},
						"metadata": {
						  "labels": {},
						  "annotations": {}
						},
						"links": {
						  "self": {
							"href": "%[1]s/v3/apps/second-test-app-guid"
						  },
						  "environment_variables": {
							"href": "%[1]s/v3/apps/second-test-app-guid/environment_variables"
						  },
						  "space": {
							"href": "%[1]s/v3/spaces/test-space-guid"
						  },
						  "processes": {
							"href": "%[1]s/v3/apps/second-test-app-guid/processes"
						  },
						  "packages": {
							"href": "%[1]s/v3/apps/second-test-app-guid/packages"
						  },
						  "current_droplet": {
							"href": "%[1]s/v3/apps/second-test-app-guid/droplets/current"
						  },
						  "droplets": {
							"href": "%[1]s/v3/apps/second-test-app-guid/droplets"
						  },
						  "tasks": {
							"href": "%[1]s/v3/apps/second-test-app-guid/tasks"
						  },
						  "start": {
							"href": "%[1]s/v3/apps/second-test-app-guid/actions/start",
							"method": "POST"
						  },
						  "stop": {
							"href": "%[1]s/v3/apps/second-test-app-guid/actions/stop",
							"method": "POST"
						  },
						  "revisions": {
							"href": "%[1]s/v3/apps/second-test-app-guid/revisions"
						  },
						  "deployed_revisions": {
							"href": "%[1]s/v3/apps/second-test-app-guid/revisions/deployed"
						  },
						  "features": {
							"href": "%[1]s/v3/apps/second-test-app-guid/features"
						  }
						}
					}
				]
			}`, defaultServerURL)), "Response body matches response:")
			})
		})

		When("no apps can be found", func() {
			BeforeEach(func() {
				appRepo.FetchAppListReturns([]repositories.AppRecord{}, nil)
			})

			It("returns status 200 OK", func() {
				Expect(rr.Code).Should(Equal(http.StatusOK), "Matching HTTP response code:")
			})

			It("returns Content-Type as JSON in header", func() {
				contentTypeHeader := rr.Header().Get("Content-Type")
				Expect(contentTypeHeader).Should(Equal(jsonHeader), "Matching Content-Type header:")
			})

			It("returns a CF API formatted Error response", func() {
				Expect(rr.Body.String()).Should(MatchJSON(fmt.Sprintf(`{
				"pagination": {
				  "total_results": 0,
				  "total_pages": 1,
				  "first": {
					"href": "%[1]s/v3/apps?page=1"
				  },
				  "last": {
					"href": "%[1]s/v3/apps?page=1"
				  },
				  "next": null,
				  "previous": null
				},
				"resources": []
			}`, defaultServerURL)), "Response body matches response:")
			})
		})

		When("there is some other error fetching apps", func() {
			BeforeEach(func() {
				appRepo.FetchAppListReturns([]repositories.AppRecord{}, errors.New("unknown!"))
			})

			itRespondsWithUnknownError(getRR)
		})
	})

	Describe("the PATCH /v3/apps/:guid/relationships/current_droplet endpoint", func() {
		const (
			dropletGUID = "test-droplet-guid"
		)

		var (
			app     repositories.AppRecord
			droplet repositories.DropletRecord
		)

		BeforeEach(func() {
			app = repositories.AppRecord{GUID: appGUID, SpaceGUID: spaceGUID}
			droplet = repositories.DropletRecord{GUID: dropletGUID, AppGUID: appGUID}

			appRepo.FetchAppReturns(app, nil)
			dropletRepo.FetchDropletReturns(droplet, nil)
			appRepo.SetCurrentDropletReturns(repositories.CurrentDropletRecord{
				AppGUID:     appGUID,
				DropletGUID: dropletGUID,
			}, nil)

			var err error
			req, err = http.NewRequest("PATCH", "/v3/apps/"+appGUID+"/relationships/current_droplet", strings.NewReader(`
					{ "data": { "guid": "`+dropletGUID+`" } }
                `))
			Expect(err).NotTo(HaveOccurred())
		})

		itDoesntSetTheCurrentDroplet := func() {
			It("doesn't set the current droplet on the app", func() {
				Expect(appRepo.SetCurrentDropletCallCount()).To(Equal(0))
			})
		}

		When("on the happy path", func() {
			It("responds with a 200 code", func() {
				Expect(rr.Code).To(Equal(200))
			})

			It("updates the k8s record via the repository", func() {
				Expect(appRepo.SetCurrentDropletCallCount()).To(Equal(1))
				_, _, message := appRepo.SetCurrentDropletArgsForCall(0)
				Expect(message.AppGUID).To(Equal(appGUID))
				Expect(message.DropletGUID).To(Equal(dropletGUID))
				Expect(message.SpaceGUID).To(Equal(spaceGUID))
			})

			It("responds with JSON", func() {
				contentTypeHeader := rr.Header().Get("Content-Type")
				Expect(contentTypeHeader).To(Equal(jsonHeader), "Matching Content-Type header:")

				Expect(rr.Body.String()).To(MatchJSON(`{
                	"data": {
                		"guid": "` + dropletGUID + `"
                	},
                	"links": {
                		"self": {
                			"href": "https://api.example.org/v3/apps/` + appGUID + `/relationships/current_droplet"
                		},
                		"related": {
                			"href": "https://api.example.org/v3/apps/` + appGUID + `/droplets/current"
                		}
                	}
                }`))
			})

			It("fetches the right App", func() {
				Expect(appRepo.FetchAppCallCount()).To(Equal(1))
				_, _, actualAppGUID := appRepo.FetchAppArgsForCall(0)
				Expect(actualAppGUID).To(Equal(appGUID))
			})

			It("fetches the right Droplet", func() {
				Expect(dropletRepo.FetchDropletCallCount()).To(Equal(1))
				_, _, actualDropletGUID := dropletRepo.FetchDropletArgsForCall(0)
				Expect(actualDropletGUID).To(Equal(dropletGUID))
			})
		})

		When("the App doesn't exist", func() {
			BeforeEach(func() {
				appRepo.FetchAppReturns(repositories.AppRecord{}, repositories.NotFoundError{})
			})

			itRespondsWithNotFound("App not found", getRR)
			itDoesntSetTheCurrentDroplet()
		})

		When("the Droplet doesn't exist", func() {
			BeforeEach(func() {
				dropletRepo.FetchDropletReturns(repositories.DropletRecord{}, repositories.NotFoundError{})
			})

			itRespondsWithUnprocessableEntity("Unable to assign current droplet. Ensure the droplet exists and belongs to this app.", getRR)
			itDoesntSetTheCurrentDroplet()
		})

		When("the Droplet belongs to a different App", func() {
			BeforeEach(func() {
				droplet.AppGUID = "a-different-app-guid"
				dropletRepo.FetchDropletReturns(repositories.DropletRecord{
					GUID:    dropletGUID,
					AppGUID: "a-different-app-guid",
				}, nil)
			})

			itRespondsWithUnprocessableEntity("Unable to assign current droplet. Ensure the droplet exists and belongs to this app.", getRR)
			itDoesntSetTheCurrentDroplet()
		})

		When("the guid is missing", func() {
			BeforeEach(func() {
				var err error
				req, err = http.NewRequest("PATCH", "/v3/apps/"+appGUID+"/relationships/current_droplet", strings.NewReader(`
					{ "data": {  } }
                `))
				Expect(err).NotTo(HaveOccurred())
			})

			itRespondsWithUnprocessableEntity("GUID is a required field", getRR)
		})

		When("building the client errors", func() {
			BeforeEach(func() {
				clientBuilder.Returns(nil, errors.New("boom"))
			})

			itRespondsWithUnknownError(getRR)
			itDoesntSetTheCurrentDroplet()
		})

		When("fetching the App errors", func() {
			BeforeEach(func() {
				appRepo.FetchAppReturns(repositories.AppRecord{}, errors.New("boom"))
			})

			itRespondsWithUnknownError(getRR)
			itDoesntSetTheCurrentDroplet()
		})

		When("fetching the Droplet errors", func() {
			BeforeEach(func() {
				dropletRepo.FetchDropletReturns(repositories.DropletRecord{}, errors.New("boom"))
			})

			itRespondsWithUnknownError(getRR)
			itDoesntSetTheCurrentDroplet()
		})

		When("setting the current droplet errors", func() {
			BeforeEach(func() {
				appRepo.SetCurrentDropletReturns(repositories.CurrentDropletRecord{}, errors.New("boom"))
			})

			itRespondsWithUnknownError(getRR)
		})
	})

	Describe("the POST /v3/apps/:guid/actions/start endpoint", func() {

		BeforeEach(func() {
			fetchAppRecord := repositories.AppRecord{
				Name:        appName,
				GUID:        appGUID,
				SpaceGUID:   spaceGUID,
				DropletGUID: "some-droplet-guid",
				State:       "STOPPED",
				Lifecycle: repositories.Lifecycle{
					Type: "buildpack",
					Data: repositories.LifecycleData{
						Buildpacks: []string{},
						Stack:      "",
					},
				},
			}
			appRepo.FetchAppReturns(fetchAppRecord, nil)
			setAppDesiredStateRecord := fetchAppRecord
			setAppDesiredStateRecord.State = "STARTED"
			appRepo.SetAppDesiredStateReturns(setAppDesiredStateRecord, nil)

			var err error
			req, err = http.NewRequest("POST", "/v3/apps/"+appGUID+"/actions/start", nil)
			Expect(err).NotTo(HaveOccurred())
		})

		When("on the happy path", func() {
			It("returns status 200 OK", func() {
				Expect(rr.Code).To(Equal(http.StatusOK), "Matching HTTP response code:")
			})

			It("returns the App in the response with a state of STARTED", func() {
				contentTypeHeader := rr.Header().Get("Content-Type")
				Expect(contentTypeHeader).To(Equal(jsonHeader), "Matching Content-Type header:")

				Expect(rr.Body.String()).To(MatchJSON(fmt.Sprintf(`{
                    "guid": "%[2]s",
                    "created_at": "",
                    "updated_at": "",
                    "name": "%[4]s",
                    "state": "STARTED",
                    "lifecycle": {
                      "type": "buildpack",
                      "data": {
                        "buildpacks": [],
                        "stack": ""
                      }
                    },
                    "relationships": {
                      "space": {
                        "data": {
                          "guid": "%[3]s"
                        }
                      }
                    },
                    "metadata": {
                      "labels": {},
                      "annotations": {}
                    },
                    "links": {
                      "self": {
                        "href": "https://api.example.org/v3/apps/%[2]s"
                      },
                      "environment_variables": {
                        "href": "https://api.example.org/v3/apps/%[2]s/environment_variables"
                      },
                      "space": {
                        "href": "https://api.example.org/v3/spaces/%[3]s"
                      },
                      "processes": {
                        "href": "https://api.example.org/v3/apps/%[2]s/processes"
                      },
                      "packages": {
                        "href": "https://api.example.org/v3/apps/%[2]s/packages"
                      },
                      "current_droplet": {
                        "href": "https://api.example.org/v3/apps/%[2]s/droplets/current"
                      },
                      "droplets": {
                        "href": "https://api.example.org/v3/apps/%[2]s/droplets"
                      },
                      "tasks": {
                        "href": "https://api.example.org/v3/apps/%[2]s/tasks"
                      },
                      "start": {
                        "href": "https://api.example.org/v3/apps/%[2]s/actions/start",
                        "method": "POST"
                      },
                      "stop": {
                        "href": "https://api.example.org/v3/apps/%[2]s/actions/stop",
                        "method": "POST"
                      },
                      "revisions": {
                        "href": "https://api.example.org/v3/apps/%[2]s/revisions"
                      },
                      "deployed_revisions": {
                        "href": "https://api.example.org/v3/apps/%[2]s/revisions/deployed"
                      },
                      "features": {
                        "href": "https://api.example.org/v3/apps/%[2]s/features"
                      }
                    }
                }`, defaultServerURL, appGUID, spaceGUID, appName)), "Response body matches response:")
			})
		})

		When("the app cannot be found", func() {
			BeforeEach(func() {
				appRepo.FetchAppReturns(repositories.AppRecord{}, repositories.NotFoundError{})
			})

			// TODO: should we return code 100004 instead?
			itRespondsWithNotFound("App not found", getRR)
		})

		When("there is some other error fetching the app", func() {
			BeforeEach(func() {
				appRepo.FetchAppReturns(repositories.AppRecord{}, errors.New("unknown!"))
			})

			itRespondsWithUnknownError(getRR)
		})

		When("the app has no droplet", func() {
			BeforeEach(func() {
				fetchAppRecord := repositories.AppRecord{
					Name:        appName,
					GUID:        appGUID,
					SpaceGUID:   spaceGUID,
					DropletGUID: "",
					State:       "STOPPED",
					Lifecycle: repositories.Lifecycle{
						Type: "buildpack",
						Data: repositories.LifecycleData{
							Buildpacks: []string{},
							Stack:      "",
						},
					},
				}
				appRepo.FetchAppReturns(fetchAppRecord, nil)
			})

			itRespondsWithUnprocessableEntity(`Assign a droplet before starting this app.`, getRR)
		})

		When("there is some other error updating app desiredState", func() {
			BeforeEach(func() {
				appRepo.SetAppDesiredStateReturns(repositories.AppRecord{}, errors.New("unknown!"))
			})

			itRespondsWithUnknownError(getRR)
		})
	})

	Describe("the GET /v3/apps/:guid/processes endpoint", func() {
		var (
			process1Record *repositories.ProcessRecord
			process2Record *repositories.ProcessRecord
		)
		BeforeEach(func() {
			processRecord := repositories.ProcessRecord{
				GUID:        "process-1-guid",
				SpaceGUID:   spaceGUID,
				AppGUID:     appGUID,
				Type:        "web",
				Command:     "rackup",
				Instances:   5,
				MemoryMB:    256,
				DiskQuotaMB: 1024,
				Ports:       []int32{8080},
				HealthCheck: repositories.HealthCheck{
					Type: "port",
					Data: repositories.HealthCheckData{
						HTTPEndpoint:             "",
						InvocationTimeoutSeconds: 0,
						TimeoutSeconds:           0,
					},
				},
				Labels:      map[string]string{},
				Annotations: map[string]string{},
				CreatedAt:   "2016-03-23T18:48:22Z",
				UpdatedAt:   "2016-03-23T18:48:42Z",
			}
			processRecord2 := processRecord
			processRecord2.GUID = "process-2-guid"
			processRecord2.Type = "worker"
			processRecord2.Instances = 1
			processRecord2.HealthCheck.Type = "process"

			process1Record = &processRecord
			process2Record = &processRecord2
			processRepo.FetchProcessesForAppReturns([]repositories.ProcessRecord{
				processRecord,
				processRecord2,
			}, nil)

			var err error
			req, err = http.NewRequest("GET", "/v3/apps/"+appGUID+"/processes", nil)
			Expect(err).NotTo(HaveOccurred())
		})
		When("On the happy path and", func() {
			When("The App has associated processes", func() {
				It("returns status 200 OK", func() {
					Expect(rr.Code).To(Equal(http.StatusOK), "Matching HTTP response code:")
				})

				It("returns the Processes in the response", func() {
					contentTypeHeader := rr.Header().Get("Content-Type")
					Expect(contentTypeHeader).To(Equal(jsonHeader), "Matching Content-Type header:")

					Expect(rr.Body.String()).Should(MatchJSON(fmt.Sprintf(`{
						"pagination": {
						  "total_results": 2,
						  "total_pages": 1,
						  "first": {
							"href": "%[1]s/v3/apps/%[2]s/processes?page=1"
						  },
						  "last": {
							"href": "%[1]s/v3/apps/%[2]s/processes?page=1"
						  },
						  "next": null,
						  "previous": null
						},
						"resources": [
							{
								"guid": "%[4]s",
								"type": "web",
								"command": "[PRIVATE DATA HIDDEN IN LISTS]",
								"instances": 5,
								"memory_in_mb": 256,
								"disk_in_mb": 1024,
								"health_check": {
									"type": "port",
									"data": {
										"timeout": null,
										"invocation_timeout": null
									}
								},
								"relationships": {
									"app": {
										"data": {
											"guid": "%[2]s"
										}
									}
								},
								"metadata": {
									"labels": {},
									"annotations": {}
								},
								"created_at": "2016-03-23T18:48:22Z",
								"updated_at": "2016-03-23T18:48:42Z",
								"links": {
									"self": {
										"href": "%[1]s/v3/processes/%[4]s"
									},
									"scale": {
										"href": "%[1]s/v3/processes/%[4]s/actions/scale",
										"method": "POST"
									},
									"app": {
										"href": "%[1]s/v3/apps/%[2]s"
									},
									"space": {
										"href": "%[1]s/v3/spaces/%[3]s"
									},
									"stats": {
										"href": "%[1]s/v3/processes/%[4]s/stats"
									}
								}
							},
							{
								"guid": "%[5]s",
								"type": "worker",
								"command": "[PRIVATE DATA HIDDEN IN LISTS]",
								"instances": 1,
								"memory_in_mb": 256,
								"disk_in_mb": 1024,
								"health_check": {
									"type": "process",
									"data": {
										"timeout": null
									}
								},
								"relationships": {
									"app": {
										"data": {
											"guid": "%[2]s"
										}
									}
								},
								"metadata": {
									"labels": {},
									"annotations": {}
								},
								"created_at": "2016-03-23T18:48:22Z",
								"updated_at": "2016-03-23T18:48:42Z",
								"links": {
									"self": {
										"href": "%[1]s/v3/processes/%[5]s"
									},
									"scale": {
										"href": "%[1]s/v3/processes/%[5]s/actions/scale",
										"method": "POST"
									},
									"app": {
										"href": "%[1]s/v3/apps/%[2]s"
									},
									"space": {
										"href": "%[1]s/v3/spaces/%[3]s"
									},
									"stats": {
										"href": "%[1]s/v3/processes/%[5]s/stats"
									}
								}
							}
						]
					}`, defaultServerURL, appGUID, spaceGUID, process1Record.GUID, process2Record.GUID)), "Response body matches response:")
				})
			})

			When("The App does not have associated processes", func() {
				BeforeEach(func() {
					processRepo.FetchProcessesForAppReturns([]repositories.ProcessRecord{}, nil)
				})

				It("returns status 200 OK", func() {
					Expect(rr.Code).To(Equal(http.StatusOK), "Matching HTTP response code:")
				})

				It("returns a response with an empty resources array", func() {
					contentTypeHeader := rr.Header().Get("Content-Type")
					Expect(contentTypeHeader).To(Equal(jsonHeader), "Matching Content-Type header:")

					Expect(rr.Body.String()).Should(MatchJSON(fmt.Sprintf(`{
						"pagination": {
						  "total_results": 0,
						  "total_pages": 1,
						  "first": {
							"href": "%[1]s/v3/apps/%[2]s/processes?page=1"
						  },
						  "last": {
							"href": "%[1]s/v3/apps/%[2]s/processes?page=1"
						  },
						  "next": null,
						  "previous": null
						},
						"resources": []
					}`, defaultServerURL, appGUID)), "Response body matches response:")
				})
			})
		})
		When("On the sad path and", func() {
			When("the app cannot be found", func() {
				BeforeEach(func() {
					appRepo.FetchAppReturns(repositories.AppRecord{}, repositories.NotFoundError{})
				})

				itRespondsWithNotFound("App not found", getRR)
			})

			When("there is some other error fetching the app", func() {
				BeforeEach(func() {
					appRepo.FetchAppReturns(repositories.AppRecord{}, errors.New("unknown!"))
				})

				itRespondsWithUnknownError(getRR)
			})
			When("there is some error fetching the app's processes", func() {
				BeforeEach(func() {
					processRepo.FetchProcessesForAppReturns([]repositories.ProcessRecord{}, errors.New("unknown!"))
				})

				itRespondsWithUnknownError(getRR)
			})
		})
	})

})

func initializeCreateAppRequestBody(appName, spaceGUID string, envVars, labels, annotations map[string]string) string {
	marshaledEnvironmentVariables, _ := json.Marshal(envVars)
	marshaledLabels, _ := json.Marshal(labels)
	marshaledAnnotations, _ := json.Marshal(annotations)

	return `{
		"name": "` + appName + `",
		"relationships": {
			"space": {
				"data": {
					"guid": "` + spaceGUID + `"
				}
			}
		},
		"environment_variables": ` + string(marshaledEnvironmentVariables) + `,
		"metadata": {
			"labels": ` + string(marshaledLabels) + `,
			"annotations": ` + string(marshaledAnnotations) + `
		}
	}`
}
