package apis_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"code.cloudfoundry.org/cf-k8s-api/apis/fake"

	logf "sigs.k8s.io/controller-runtime/pkg/log"

	. "code.cloudfoundry.org/cf-k8s-api/apis"
	"code.cloudfoundry.org/cf-k8s-api/repositories"
	. "github.com/onsi/gomega"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"k8s.io/client-go/rest"
)

func TestApps(t *testing.T) {
	spec.Run(t, "AppGetHandler", testAppsGetHandler, spec.Report(report.Terminal{}))
	spec.Run(t, "AppCreateHandler", testAppsCreateHandler, spec.Report(report.Terminal{}))
}

func testAppsGetHandler(t *testing.T, when spec.G, it spec.S) {
	Expect := NewWithT(t).Expect

	const (
		testAppHandlerLoggerName = "TestAppHandler"
	)

	var (
		rr         *httptest.ResponseRecorder
		req        *http.Request
		appRepo    *fake.CFAppRepository
		apiHandler *AppHandler
	)

	it.Before(func() {
		appRepo = new(fake.CFAppRepository)
		appRepo.FetchAppReturns(repositories.AppRecord{
			GUID:      "test-app-guid",
			Name:      "test-app",
			SpaceGUID: "test-space-guid",
			State:     "STOPPED",
			Lifecycle: repositories.Lifecycle{
				Data: repositories.LifecycleData{
					Buildpacks: []string{},
					Stack:      "",
				},
			},
		}, nil)

		var err error
		req, err = http.NewRequest("GET", "/v3/apps/my-app-guid", nil)
		Expect(err).NotTo(HaveOccurred())

		rr = httptest.NewRecorder()
		clientBuilder := new(fake.ClientBuilder)

		apiHandler = &AppHandler{
			ServerURL:   defaultServerURL,
			AppRepo:     appRepo,
			Logger:      logf.Log.WithName(testAppHandlerLoggerName),
			K8sConfig:   &rest.Config{},
			BuildClient: clientBuilder.Spy,
		}
	})

	when("the GET /v3/apps/:guid  endpoint returns successfully", func() {
		it.Before(func() {
			http.HandlerFunc(apiHandler.AppGetHandler).ServeHTTP(rr, req)
		})

		it("returns status 200 OK", func() {
			httpStatus := rr.Code
			Expect(httpStatus).Should(Equal(http.StatusOK), "Matching HTTP response code:")
		})

		it("returns Content-Type as JSON in header", func() {
			contentTypeHeader := rr.Header().Get("Content-Type")
			Expect(contentTypeHeader).Should(Equal(jsonHeader), "Matching Content-Type header:")
		})

		it("returns the App in the response", func() {
			expectedBody := `{
				"name": "test-app",
				"guid": "test-app-guid",
				"state": "STOPPED",
				"created_at": "",
				"updated_at": "",
				"relationships": {
					"space": {
						"data": {
							"guid": "test-space-guid"
						}
					}
				},
				"lifecycle": {
					"data": {
						"buildpacks": [],
						"stack": ""
					}
				},
				"metadata": {
					"labels": {},
					"annotations": {}
				},
				"links": {
					"self": {
						"href": "https://api.example.org/v3/apps/test-app-guid"
					},
					"space": {
						"href": "https://api.example.org/v3/spaces/test-space-guid"
					},
					"processes": {
						"href": "https://api.example.org/v3/apps/test-app-guid/processes"
					},
					"packages": {
						"href": "https://api.example.org/v3/apps/test-app-guid/packages"
					},
					"environment_variables": {
						"href": "https://api.example.org/v3/apps/test-app-guid/environment_variables"
				 	},
					"current_droplet": {
						"href": "https://api.example.org/v3/apps/test-app-guid/droplets/current"
					},
					"droplets": {
						"href": "https://api.example.org/v3/apps/test-app-guid/droplets"
					},
					"tasks": {},
				    "start": {
						"href": "https://api.example.org/v3/apps/test-app-guid/actions/start",
						"method": "POST"
					},
					"stop": {
						"href": "https://api.example.org/v3/apps/test-app-guid/actions/stop",
				   		"method": "POST"
					},
					"revisions": {},
					"deployed_revisions": {},
					"features": {}
				}
            }`

			Expect(rr.Body.String()).Should(MatchJSON(expectedBody), "Response body matches response:")
		})
	})

	when("the app cannot be found", func() {
		it.Before(func() {
			appRepo.FetchAppReturns(repositories.AppRecord{}, repositories.NotFoundError{Err: errors.New("not found")})

			http.HandlerFunc(apiHandler.AppGetHandler).ServeHTTP(rr, req)
		})

		it("returns a CF API formatted Error response", func() {
			Expect(rr.Code).Should(Equal(http.StatusNotFound), "Matching HTTP response code:")

			expectedBody := `{
				"errors": [
					{
						"code": 10010,
						"title": "CF-ResourceNotFound",
						"detail": "App not found"
					}
				]
            }`
			Expect(rr.Body.String()).Should(MatchJSON(expectedBody), "Response body matches response:")
		})
	})

	when("there is some other error fetching the app", func() {
		it.Before(func() {
			appRepo.FetchAppReturns(repositories.AppRecord{}, errors.New("unknown!"))

			http.HandlerFunc(apiHandler.AppGetHandler).ServeHTTP(rr, req)
		})

		it("returns a CF API formatted Error response", func() {
			Expect(rr.Code).Should(Equal(http.StatusInternalServerError), "Matching HTTP response code:")

			expectedBody := `{
				"errors": [
					{
						"code": 10001,
						"title": "UnknownError",
						"detail": "An unknown error occurred."
					}
				]
            }`
			Expect(rr.Body.String()).Should(MatchJSON(expectedBody), "Response body matches response:")
		})
	})

}

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

func testAppsCreateHandler(t *testing.T, when spec.G, it spec.S) {
	Expect := NewWithT(t).Expect

	const (
		jsonHeader       = "application/json"
		defaultServerURL = "https://api.example.org"
		testAppName      = "test-app"
		testSpaceGUID    = "test-space-guid"

		testAppHandlerLoggerName = "TestAppHandler"
	)

	var (
		rr         *httptest.ResponseRecorder
		apiHandler *AppHandler
		appRepo    *fake.CFAppRepository
	)

	makePostRequest := func(requestBody string) {
		req, err := http.NewRequest("POST", "/v3/apps", strings.NewReader(requestBody))
		Expect(err).NotTo(HaveOccurred())

		handler := http.HandlerFunc(apiHandler.AppCreateHandler)
		handler.ServeHTTP(rr, req)
	}

	when("the POST /v3/apps endpoint is invoked and", func() {
		it.Before(func() {
			appRepo = new(fake.CFAppRepository)
			apiHandler = &AppHandler{
				ServerURL:   defaultServerURL,
				AppRepo:     appRepo,
				Logger:      logf.Log.WithName(testAppHandlerLoggerName),
				K8sConfig:   &rest.Config{},
				BuildClient: new(fake.ClientBuilder).Spy,
			}
			rr = httptest.NewRecorder()
		})

		when("the request body is invalid", func() {
			it.Before(func() {
				makePostRequest(`{"description" : "Invalid Request"}`)
			})

			it("returns a status 400 Bad Request ", func() {
				Expect(rr.Code).To(Equal(http.StatusBadRequest))

			})
			it("has the expected error response body", func() {
				Expect(rr.Body).To(MatchJSON(`{
					"errors": [
						{
							"code": 1001,
							"title": "CF-MessageParseError",
							"detail": "Request invalid due to parse error: invalid request body"
						}
					]
        		}`))
			})

		})

		when("the request body is invalid with invalid app name", func() {
			it.Before(func() {
				makePostRequest(`{
					"name": 12345,
					"relationships": {
						"space": {
							"data": {
								"guid": "2f35885d-0c9d-4423-83ad-fd05066f8576"
							}
						}
					}
				}`)
			})

			it("returns a status 422 Unprocessable Entity", func() {
				Expect(rr.Code).To(Equal(http.StatusUnprocessableEntity))
			})

			it("has the expected error response body", func() {
				Expect(rr.Body).To(MatchJSON(`{
                  "errors": [
                    {
				 	  "code":   10008,
				      "title":  "CF-UnprocessableEntity",
				   	  "detail": "Name must be a string"
                    }
                  ]
				}`))
			})

		})

		when("the request body is invalid with invalid environment variable object", func() {
			it.Before(func() {
				makePostRequest(`{
					"name": "my_app",
					"environment_variables": [],
					"relationships": {
						"space": {
							"data": {
								"guid": "2f35885d-0c9d-4423-83ad-fd05066f8576"
							}
						}
					}
				}`)
			})

			it("returns a status 422 Unprocessable Entity", func() {
				Expect(rr.Code).To(Equal(http.StatusUnprocessableEntity))

			})

			it("has the expected error response body", func() {
				Expect(rr.Body).To(MatchJSON(`{
					"errors": [
						{
							"code": 10008,
							"title": "CF-UnprocessableEntity",
							"detail": "Environment_variables must be a map[string]string"
						}
					]
        		}`))
			})
		})

		when("the request body is invalid with missing required name field", func() {
			it.Before(func() {
				makePostRequest(`{
					"relationships": {
						"space": {
							"data": {
								"guid": "0c78dd5d-c723-4f2e-b168-df3c3e1d0806"
							}
					 	}
					}
				}`)
			})

			it("returns a status 422 Unprocessable Entity", func() {
				Expect(rr.Code).To(Equal(http.StatusUnprocessableEntity))
			})

			it("has the expected error response body", func() {
				Expect(rr.Body).To(MatchJSON(`{
					"errors": [
						{
							"code": 10008,
							"title": "CF-UnprocessableEntity",
							"detail": "Name must be a string"
						}
					]
        		}`))
			})
		})

		when("the request body is invalid with missing data within lifecycle", func() {
			it.Before(func() {
				makePostRequest(`{
					"name": "test-app",
					"lifecycle":{},
					"relationships": {
						"space": {
							"data": {
								"guid": "0c78dd5d-c723-4f2e-b168-df3c3e1d0806"
							 }
					 	 }
					}
				}`)

			})

			it("returns a status 422 Unprocessable Entity", func() {
				Expect(rr.Code).To(Equal(http.StatusUnprocessableEntity))
			})

			it("has the expected error response body", func() {
				Expect(rr.Body).To(MatchJSON(`{
					"errors": [
						{
							"code": 10008,
							"title": "CF-UnprocessableEntity",
							"detail": "Type must be a string,Buildpacks must be a []string,Stack must be a string"
						}
					]
        		}`))
			})
		})

		when("the space does not exist", func() {
			it.Before(func() {
				appRepo.FetchNamespaceReturns(repositories.SpaceRecord{},
					repositories.PermissionDeniedOrNotFoundError{Err: errors.New("not found")})

				requestBody := initializeCreateAppRequestBody(testAppName, "no-such-guid", nil, nil, nil)
				makePostRequest(requestBody)
			})

			it("returns a CF API formatted Error response", func() {
				httpStatus := rr.Code
				Expect(httpStatus).Should(Equal(http.StatusUnprocessableEntity), "Matching HTTP response code:")

				expectedBody := `{
					"errors": [
						{
							"code": 10008,
							"title": "CF-UnprocessableEntity",
							"detail": "Invalid space. Ensure that the space exists and you have access to it."
						}
					]
        		}`
				Expect(rr.Body.String()).Should(MatchJSON(expectedBody), "Response body matches response:")
			})
		})

		when("the app already exists", func() {
			it.Before(func() {
				appRepo.AppExistsReturns(true, nil)

				requestBody := initializeCreateAppRequestBody(testAppName, testSpaceGUID, nil, nil, nil)
				makePostRequest(requestBody)
			})

			it("returns a CF API formatted Error response", func() {
				httpStatus := rr.Code
				Expect(httpStatus).Should(Equal(http.StatusUnprocessableEntity), "Matching HTTP response code:")

				expectedBody := `{
					"errors": [
						{
							"code": 10016,
							"title": "CF-UniquenessError",
							"detail": "App with the name 'test-app' already exists."
						}
					]
        		}`
				Expect(rr.Body.String()).Should(MatchJSON(expectedBody), "Response body matches response:")
			})
		})

		when("the namespace exists and app does not exist and", func() {
			it.Before(func() {
				appRepo.AppExistsReturns(false, nil)
			})

			when("a plain POST test app request is sent without env vars or metadata", func() {
				const testAppGUID = "test-app-guid"

				it.Before(func() {
					appRepo.CreateAppReturns(repositories.AppRecord{
						GUID:      testAppGUID,
						Name:      testAppName,
						SpaceGUID: testSpaceGUID,
						State:     repositories.DesiredState("STOPPED"),
						Lifecycle: repositories.Lifecycle{
							Data: repositories.LifecycleData{
								Buildpacks: []string{},
								Stack:      "",
							},
						},
					}, nil)

					requestBody := initializeCreateAppRequestBody(testAppName, testSpaceGUID, nil, nil, nil)
					makePostRequest(requestBody)
				})

				it("should invoke repo CreateApp with a random GUID", func() {
					Expect(appRepo.CreateAppCallCount()).To(Equal(1), "Repo CreateApp count was not invoked 1 time")
					_, _, createAppRecord := appRepo.CreateAppArgsForCall(0)
					Expect(createAppRecord.GUID).To(MatchRegexp("^[-0-9a-f]{36}$"), "CreateApp record GUID was not a 36 character guid")
				})

				it("should not invoke repo CreateAppEnvironmentVariables when no environment variables are provided", func() {
					Expect(appRepo.CreateAppEnvironmentVariablesCallCount()).To(BeZero(), "Repo CreateAppEnvironmentVariables was invoked even though no environment vars were provided")
				})

				it("return status 200OK", func() {
					httpStatus := rr.Code
					Expect(httpStatus).Should(Equal(http.StatusOK), "Matching HTTP response code:")
				})

				it("returns Content-Type as JSON in header", func() {
					contentTypeHeader := rr.Header().Get("Content-Type")
					Expect(contentTypeHeader).Should(Equal(jsonHeader), "Matching Content-Type header:")
				})

				it(`returns the "created app" (the mock response record) in the response`, func() {
					expectedBody := `{
						"name": "test-app",
						"guid": "test-app-guid",
						"state": "STOPPED",
						"created_at": "",
						"updated_at": "",
						"relationships": {
							"space": {
								"data": {
						    		"guid": "test-space-guid"
						  		}
							}
						},
						"lifecycle": {
							"data": {
								"buildpacks": [],
								"stack": ""
							}
						},
						"metadata": {
							"labels": {},
							"annotations": {}
						},
						"links": {
							"self": {
								"href": "https://api.example.org/v3/apps/test-app-guid"
						 	},
							"space": {
								"href": "https://api.example.org/v3/spaces/test-space-guid"
							},
							"processes": {
								"href": "https://api.example.org/v3/apps/test-app-guid/processes"
							},
							"packages": {
								"href": "https://api.example.org/v3/apps/test-app-guid/packages"
							},
							"environment_variables": {
								"href": "https://api.example.org/v3/apps/test-app-guid/environment_variables"
							},
							"current_droplet": {
								"href": "https://api.example.org/v3/apps/test-app-guid/droplets/current"
							},
							"droplets": {
								"href": "https://api.example.org/v3/apps/test-app-guid/droplets"
							},
							"tasks": {},
							"start": {
								"href": "https://api.example.org/v3/apps/test-app-guid/actions/start",
								"method": "POST"
							},
							"stop": {
								"href": "https://api.example.org/v3/apps/test-app-guid/actions/stop",
								"method": "POST"
							},
							"revisions": {},
							"deployed_revisions": {},
							"features": {}
						}
					}`
					Expect(rr.Body.String()).Should(MatchJSON(expectedBody), "Response body matches response:")
				})
			})

			when("a POST test app request is sent with env vars and", func() {
				var (
					testEnvironmentVariables map[string]string
					requestBody              string
				)

				it.Before(func() {
					testEnvironmentVariables = map[string]string{"foo": "foo", "bar": "bar"}

					requestBody = initializeCreateAppRequestBody(testAppName, testSpaceGUID, testEnvironmentVariables, nil, nil)
				})

				when("the env var repository is working and will not return an error", func() {
					const createEnvVarsResponseName = "testAppGUID-env"

					it.Before(func() {
						appRepo.CreateAppEnvironmentVariablesReturns(repositories.AppEnvVarsRecord{
							Name: createEnvVarsResponseName,
						}, nil)

						makePostRequest(requestBody)
					})

					it("should call Repo CreateAppEnvironmentVariables with the space and environment vars", func() {
						Expect(appRepo.CreateAppEnvironmentVariablesCallCount()).To(Equal(1), "Repo CreateAppEnvironmentVariables count was not invoked 1 time")
						_, _, createAppEnvVarsRecord := appRepo.CreateAppEnvironmentVariablesArgsForCall(0)
						Expect(createAppEnvVarsRecord.EnvironmentVariables).To(Equal(testEnvironmentVariables))
						Expect(createAppEnvVarsRecord.SpaceGUID).To(Equal(testSpaceGUID))
					})

					it("should call Repo CreateApp and provide the name of the created env Secret", func() {
						Expect(appRepo.CreateAppCallCount()).To(Equal(1), "Repo CreateApp count was not invoked 1 time")
						_, _, createAppRecord := appRepo.CreateAppArgsForCall(0)
						Expect(createAppRecord.EnvSecretName).To(Equal(createEnvVarsResponseName))
					})
				})

				when("there will be a repository error with creating the env vars", func() {
					it.Before(func() {
						appRepo.CreateAppEnvironmentVariablesReturns(repositories.AppEnvVarsRecord{}, errors.New("intentional error"))

						makePostRequest(requestBody)
					})

					it("should return an error", func() {
						Expect(rr.Code).To(Equal(http.StatusInternalServerError))
					})
				})
			})

			when("a POST test app request is sent with metadata labels", func() {
				var (
					testLabels map[string]string
				)

				it.Before(func() {
					testLabels = map[string]string{"foo": "foo", "bar": "bar"}

					requestBody := initializeCreateAppRequestBody(testAppName, testSpaceGUID, nil, testLabels, nil)
					makePostRequest(requestBody)
				})

				it("should pass along the labels to CreateApp", func() {
					Expect(appRepo.CreateAppCallCount()).To(Equal(1), "Repo CreateApp count was not invoked 1 time")
					_, _, createAppRecord := appRepo.CreateAppArgsForCall(0)
					Expect(createAppRecord.Labels).To(Equal(testLabels))
				})
			})

			when("a POST test app request is sent with metadata annotations", func() {
				var (
					testAnnotations map[string]string
				)

				it.Before(func() {
					testAnnotations = map[string]string{"foo": "foo", "bar": "bar"}
					requestBody := initializeCreateAppRequestBody(testAppName, testSpaceGUID, nil, nil, testAnnotations)
					makePostRequest(requestBody)
				})

				it("should pass along the annotations to CreateApp", func() {
					Expect(appRepo.CreateAppCallCount()).To(Equal(1), "Repo CreateApp count was not invoked 1 time")
					_, _, createAppRecord := appRepo.CreateAppArgsForCall(0)
					Expect(createAppRecord.Annotations).To(Equal(testAnnotations))
				})
			})
		})
	})
}
