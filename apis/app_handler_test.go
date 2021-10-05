package apis_test

import (
	"encoding/json"
	"errors"
	"github.com/gorilla/mux"
	"net/http"
	"net/http/httptest"
	"strings"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"

	. "code.cloudfoundry.org/cf-k8s-api/apis"
	"code.cloudfoundry.org/cf-k8s-api/apis/fake"
	"code.cloudfoundry.org/cf-k8s-api/repositories"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"k8s.io/client-go/rest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	appGUID   = "test-app-guid"
	spaceGUID = "test-space-guid"
)

var _ = Describe("AppHandler", func() {
	Describe("the GET /v3/apps/:guid endpoint", func() {
		const (
			testAppHandlerLoggerName = "TestAppHandler"
		)

		var (
			rr      *httptest.ResponseRecorder
			req     *http.Request
			appRepo *fake.CFAppRepository
			router  *mux.Router
		)

		getRR := func() *httptest.ResponseRecorder { return rr }

		BeforeEach(func() {
			appRepo = new(fake.CFAppRepository)
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

			rr = httptest.NewRecorder()
			router = mux.NewRouter()
			clientBuilder := new(fake.ClientBuilder)

			apiHandler := NewAppHandler(
				logf.Log.WithName(testAppHandlerLoggerName),
				defaultServerURL,
				appRepo,
				clientBuilder.Spy,
				&rest.Config{},
			)
			apiHandler.RegisterRoutes(router)
		})

		When("on the happy path", func() {
			BeforeEach(func() {
				router.ServeHTTP(rr, req)
			})

			It("returns status 200 OK", func() {
				Expect(rr.Code).To(Equal(http.StatusOK), "Matching HTTP response code:")
			})

			It("returns Content-Type as JSON in header", func() {
				contentTypeHeader := rr.Header().Get("Content-Type")
				Expect(contentTypeHeader).To(Equal(jsonHeader), "Matching Content-Type header:")
			})

			It("returns the App in the response", func() {
				Expect(rr.Body.String()).To(MatchJSON(`{
				"guid": "`+appGUID+`",
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
					  "guid": "`+spaceGUID+`"
					}
				  }
				},
				"metadata": {
				  "labels": {},
				  "annotations": {}
				},
				"links": {
				  "self": {
					"href": "https://api.example.org/v3/apps/`+appGUID+`"
				  },
				  "environment_variables": {
					"href": "https://api.example.org/v3/apps/`+appGUID+`/environment_variables"
				  },
				  "space": {
					"href": "https://api.example.org/v3/spaces/`+spaceGUID+`"
				  },
				  "processes": {
					"href": "https://api.example.org/v3/apps/`+appGUID+`/processes"
				  },
				  "packages": {
					"href": "https://api.example.org/v3/apps/`+appGUID+`/packages"
				  },
				  "current_droplet": {
					"href": "https://api.example.org/v3/apps/`+appGUID+`/droplets/current"
				  },
				  "droplets": {
					"href": "https://api.example.org/v3/apps/`+appGUID+`/droplets"
				  },
				  "tasks": {
					"href": "https://api.example.org/v3/apps/`+appGUID+`/tasks"
				  },
				  "start": {
					"href": "https://api.example.org/v3/apps/`+appGUID+`/actions/start",
					"method": "POST"
				  },
				  "stop": {
					"href": "https://api.example.org/v3/apps/`+appGUID+`/actions/stop",
					"method": "POST"
				  },
				  "revisions": {
					"href": "https://api.example.org/v3/apps/`+appGUID+`/revisions"
				  },
				  "deployed_revisions": {
					"href": "https://api.example.org/v3/apps/`+appGUID+`/revisions/deployed"
				  },
				  "features": {
					"href": "https://api.example.org/v3/apps/`+appGUID+`/features"
				  }
				}
			}`), "Response body matches response:")
			})
		})

		When("the app cannot be found", func() {
			BeforeEach(func() {
				appRepo.FetchAppReturns(repositories.AppRecord{}, repositories.NotFoundError{})

				router.ServeHTTP(rr, req)
			})

			// TODO: should we return code 100004 instead?
			itRespondsWithNotFound("App not found", getRR)
		})

		When("there is some other error fetching the app", func() {
			BeforeEach(func() {
				appRepo.FetchAppReturns(repositories.AppRecord{}, errors.New("unknown!"))

				router.ServeHTTP(rr, req)
			})

			itRespondsWithUnknownError(getRR)
		})
	})
	Describe("the POST /v3/apps endpoint", func() {
		const (
			testAppName = "test-app"

			testAppHandlerLoggerName = "TestAppHandler"
		)

		var (
			rr      *httptest.ResponseRecorder
			appRepo *fake.CFAppRepository
			router  *mux.Router
		)

		getRR := func() *httptest.ResponseRecorder { return rr }

		makePostRequest := func(requestBody string) {
			req, err := http.NewRequest("POST", "/v3/apps", strings.NewReader(requestBody))
			Expect(err).NotTo(HaveOccurred())

			router.ServeHTTP(rr, req)
		}

		BeforeEach(func() {
			rr = httptest.NewRecorder()
			router = mux.NewRouter()

			appRepo = new(fake.CFAppRepository)
			apiHandler := NewAppHandler(
				logf.Log.WithName(testAppHandlerLoggerName),
				defaultServerURL,
				appRepo,
				new(fake.ClientBuilder).Spy,
				&rest.Config{},
			)
			apiHandler.RegisterRoutes(router)
		})

		When("the request body is invalid json", func() {
			BeforeEach(func() {
				makePostRequest(`{`)
			})

			It("returns a status 400 Bad Request ", func() {
				Expect(rr.Code).To(Equal(http.StatusBadRequest), "Matching HTTP response code:")
			})

			It("returns Content-Type as JSON in header", func() {
				contentTypeHeader := rr.Header().Get("Content-Type")
				Expect(contentTypeHeader).To(Equal(jsonHeader), "Matching Content-Type header:")
			})

			It("has the expected error response body", func() {
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
				makePostRequest(`{"description" : "Invalid Request"}`)
			})

			It("returns a status 422 Bad Request ", func() {
				Expect(rr.Code).To(Equal(http.StatusUnprocessableEntity), "Matching HTTP response code:")
			})

			It("returns Content-Type as JSON in header", func() {
				contentTypeHeader := rr.Header().Get("Content-Type")
				Expect(contentTypeHeader).To(Equal(jsonHeader), "Matching Content-Type header:")
			})

			It("has the expected error response body", func() {
				Expect(rr.Body.String()).To(MatchJSON(`{
				"errors": [
					{
						"title": "CF-UnprocessableEntity",
						"detail": "invalid request body: json: unknown field \"description\"",
						"code": 10008
					}
				]
			}`), "Response body matches response:")
			})
		})

		When("the request body is invalid with invalid app name", func() {
			BeforeEach(func() {
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

			It("returns a status 422 Unprocessable Entity", func() {
				Expect(rr.Code).To(Equal(http.StatusUnprocessableEntity), "Matching HTTP response code:")
			})

			It("returns Content-Type as JSON in header", func() {
				contentTypeHeader := rr.Header().Get("Content-Type")
				Expect(contentTypeHeader).To(Equal(jsonHeader), "Matching Content-Type header:")
			})

			It("has the expected error response body", func() {
				Expect(rr.Body.String()).To(MatchJSON(`{
				"errors": [
					{
						"code":   10008,
						"title": "CF-UnprocessableEntity",
						"detail": "Name must be a string"
					}
				]
			}`), "Response body matches response:")
			})
		})

		When("the request body is invalid with invalid environment variable object", func() {
			BeforeEach(func() {
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

			It("returns a status 422 Unprocessable Entity", func() {
				Expect(rr.Code).To(Equal(http.StatusUnprocessableEntity), "Matching HTTP response code:")
			})

			It("returns Content-Type as JSON in header", func() {
				contentTypeHeader := rr.Header().Get("Content-Type")
				Expect(contentTypeHeader).To(Equal(jsonHeader), "Matching Content-Type header:")
			})

			It("has the expected error response body", func() {
				Expect(rr.Body.String()).To(MatchJSON(`{
				"errors": [
					{
						"title": "CF-UnprocessableEntity",
						"detail": "Environment_variables must be a map[string]string",
						"code": 10008
					}
				]
			}`), "Response body matches response:")
			})
		})

		When("the request body is invalid with missing required name field", func() {
			BeforeEach(func() {
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

			It("returns a status 422 Unprocessable Entity", func() {
				Expect(rr.Code).To(Equal(http.StatusUnprocessableEntity), "Matching HTTP response code:")
			})

			It("returns Content-Type as JSON in header", func() {
				contentTypeHeader := rr.Header().Get("Content-Type")
				Expect(contentTypeHeader).To(Equal(jsonHeader), "Matching Content-Type header:")
			})

			It("has the expected error response body", func() {
				Expect(rr.Body.String()).To(MatchJSON(`{
				"errors": [
					{
						"title": "CF-UnprocessableEntity",
						"detail": "Name is a required field",
						"code": 10008
					}
				]
			}`), "Response body matches response:")
			})
		})

		When("the request body is invalid with missing data within lifecycle", func() {
			BeforeEach(func() {
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

			It("returns a status 422 Unprocessable Entity", func() {
				Expect(rr.Code).To(Equal(http.StatusUnprocessableEntity), "Matching HTTP response code:")
			})

			It("returns Content-Type as JSON in header", func() {
				contentTypeHeader := rr.Header().Get("Content-Type")
				Expect(contentTypeHeader).To(Equal(jsonHeader), "Matching Content-Type header:")
			})

			It("has the expected error response body", func() {
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
				appRepo.FetchNamespaceReturns(repositories.SpaceRecord{},
					repositories.PermissionDeniedOrNotFoundError{Err: errors.New("not found")})

				requestBody := initializeCreateAppRequestBody(testAppName, "no-such-guid", nil, nil, nil)
				makePostRequest(requestBody)
			})

			It("returns a status 422 Unprocessable Entity", func() {
				Expect(rr.Code).To(Equal(http.StatusUnprocessableEntity), "Matching HTTP response code:")
			})

			It("returns Content-Type as JSON in header", func() {
				contentTypeHeader := rr.Header().Get("Content-Type")
				Expect(contentTypeHeader).To(Equal(jsonHeader), "Matching Content-Type header:")
			})

			It("returns a CF API formatted Error response", func() {
				Expect(rr.Body.String()).To(MatchJSON(`{
				"errors": [
					{
						"title": "CF-UnprocessableEntity",
						"detail": "Invalid space. Ensure that the space exists and you have access to it.",
						"code": 10008
					}
				]
			}`), "Response body matches response:")
			})
		})

		When("the app already exists, but AppCreate returns false due to validating webhook rejection", func() {
			BeforeEach(func() {
				controllerError := new(k8serrors.StatusError)
				controllerError.ErrStatus.Reason = `{"code":1,"message":"CFApp with the same spec.name exists"}`
				appRepo.CreateAppReturns(repositories.AppRecord{}, controllerError)

				requestBody := initializeCreateAppRequestBody(testAppName, spaceGUID, nil, nil, nil)
				makePostRequest(requestBody)
			})

			It("returns a status 422 Unprocessable Entity", func() {
				Expect(rr.Code).To(Equal(http.StatusUnprocessableEntity), "Matching HTTP response code:")
			})

			It("returns Content-Type as JSON in header", func() {
				contentTypeHeader := rr.Header().Get("Content-Type")
				Expect(contentTypeHeader).To(Equal(jsonHeader), "Matching Content-Type header:")
			})

			It("returns a CF API formatted Error response", func() {
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
				makePostRequest(requestBody)
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
					makePostRequest(requestBody)
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
					Expect(rr.Body.String()).To(MatchJSON(`{
					"guid": "`+appGUID+`",
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
						  "guid": "`+spaceGUID+`"
						}
					  }
					},
					"metadata": {
					  "labels": {},
					  "annotations": {}
					},
					"links": {
					  "self": {
						"href": "https://api.example.org/v3/apps/`+appGUID+`"
					  },
					  "environment_variables": {
						"href": "https://api.example.org/v3/apps/`+appGUID+`/environment_variables"
					  },
					  "space": {
						"href": "https://api.example.org/v3/spaces/`+spaceGUID+`"
					  },
					  "processes": {
						"href": "https://api.example.org/v3/apps/`+appGUID+`/processes"
					  },
					  "packages": {
						"href": "https://api.example.org/v3/apps/`+appGUID+`/packages"
					  },
					  "current_droplet": {
						"href": "https://api.example.org/v3/apps/`+appGUID+`/droplets/current"
					  },
					  "droplets": {
						"href": "https://api.example.org/v3/apps/`+appGUID+`/droplets"
					  },
					  "tasks": {
						"href": "https://api.example.org/v3/apps/`+appGUID+`/tasks"
					  },
					  "start": {
						"href": "https://api.example.org/v3/apps/`+appGUID+`/actions/start",
						"method": "POST"
					  },
					  "stop": {
						"href": "https://api.example.org/v3/apps/`+appGUID+`/actions/stop",
						"method": "POST"
					  },
					  "revisions": {
						"href": "https://api.example.org/v3/apps/`+appGUID+`/revisions"
					  },
					  "deployed_revisions": {
						"href": "https://api.example.org/v3/apps/`+appGUID+`/revisions/deployed"
					  },
					  "features": {
						"href": "https://api.example.org/v3/apps/`+appGUID+`/features"
					  }
					}
				}`), "Response body matches response:")
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

						makePostRequest(requestBody)
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

						makePostRequest(requestBody)
					})

					itRespondsWithUnknownError(getRR)
				})
			})

			When("a POST test app request is sent with metadata labels", func() {
				var testLabels map[string]string

				BeforeEach(func() {
					testLabels = map[string]string{"foo": "foo", "bar": "bar"}

					requestBody := initializeCreateAppRequestBody(testAppName, spaceGUID, nil, testLabels, nil)
					makePostRequest(requestBody)
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
					makePostRequest(requestBody)
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
		const (
			testAppHandlerLoggerName = "TestAppHandler"
		)

		var (
			rr      *httptest.ResponseRecorder
			req     *http.Request
			router  *mux.Router
			appRepo *fake.CFAppRepository
		)

		getRR := func() *httptest.ResponseRecorder { return rr }

		BeforeEach(func() {
			appRepo = new(fake.CFAppRepository)
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

			rr = httptest.NewRecorder()
			router = mux.NewRouter()
			clientBuilder := new(fake.ClientBuilder)

			apiHandler := NewAppHandler(
				logf.Log.WithName(testAppHandlerLoggerName),
				defaultServerURL,
				appRepo,
				clientBuilder.Spy,
				&rest.Config{},
			)
			apiHandler.RegisterRoutes(router)
		})

		When("on the happy path", func() {
			BeforeEach(func() {
				router.ServeHTTP(rr, req)
			})

			It("returns status 200 OK", func() {
				Expect(rr.Code).Should(Equal(http.StatusOK), "Matching HTTP response code:")
			})

			It("returns Content-Type as JSON in header", func() {
				contentTypeHeader := rr.Header().Get("Content-Type")
				Expect(contentTypeHeader).Should(Equal(jsonHeader), "Matching Content-Type header:")
			})

			It("returns the Pagination Data and App Resources in the response", func() {
				Expect(rr.Body.String()).Should(MatchJSON(`{
				"pagination": {
				  "total_results": 2,
				  "total_pages": 1,
				  "first": {
					"href": "https://api.example.org/v3/apps?page=1"
				  },
				  "last": {
					"href": "https://api.example.org/v3/apps?page=1"
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
							"href": "https://api.example.org/v3/apps/first-test-app-guid"
						  },
						  "environment_variables": {
							"href": "https://api.example.org/v3/apps/first-test-app-guid/environment_variables"
						  },
						  "space": {
							"href": "https://api.example.org/v3/spaces/test-space-guid"
						  },
						  "processes": {
							"href": "https://api.example.org/v3/apps/first-test-app-guid/processes"
						  },
						  "packages": {
							"href": "https://api.example.org/v3/apps/first-test-app-guid/packages"
						  },
						  "current_droplet": {
							"href": "https://api.example.org/v3/apps/first-test-app-guid/droplets/current"
						  },
						  "droplets": {
							"href": "https://api.example.org/v3/apps/first-test-app-guid/droplets"
						  },
						  "tasks": {
							"href": "https://api.example.org/v3/apps/first-test-app-guid/tasks"
						  },
						  "start": {
							"href": "https://api.example.org/v3/apps/first-test-app-guid/actions/start",
							"method": "POST"
						  },
						  "stop": {
							"href": "https://api.example.org/v3/apps/first-test-app-guid/actions/stop",
							"method": "POST"
						  },
						  "revisions": {
							"href": "https://api.example.org/v3/apps/first-test-app-guid/revisions"
						  },
						  "deployed_revisions": {
							"href": "https://api.example.org/v3/apps/first-test-app-guid/revisions/deployed"
						  },
						  "features": {
							"href": "https://api.example.org/v3/apps/first-test-app-guid/features"
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
							"href": "https://api.example.org/v3/apps/second-test-app-guid"
						  },
						  "environment_variables": {
							"href": "https://api.example.org/v3/apps/second-test-app-guid/environment_variables"
						  },
						  "space": {
							"href": "https://api.example.org/v3/spaces/test-space-guid"
						  },
						  "processes": {
							"href": "https://api.example.org/v3/apps/second-test-app-guid/processes"
						  },
						  "packages": {
							"href": "https://api.example.org/v3/apps/second-test-app-guid/packages"
						  },
						  "current_droplet": {
							"href": "https://api.example.org/v3/apps/second-test-app-guid/droplets/current"
						  },
						  "droplets": {
							"href": "https://api.example.org/v3/apps/second-test-app-guid/droplets"
						  },
						  "tasks": {
							"href": "https://api.example.org/v3/apps/second-test-app-guid/tasks"
						  },
						  "start": {
							"href": "https://api.example.org/v3/apps/second-test-app-guid/actions/start",
							"method": "POST"
						  },
						  "stop": {
							"href": "https://api.example.org/v3/apps/second-test-app-guid/actions/stop",
							"method": "POST"
						  },
						  "revisions": {
							"href": "https://api.example.org/v3/apps/second-test-app-guid/revisions"
						  },
						  "deployed_revisions": {
							"href": "https://api.example.org/v3/apps/second-test-app-guid/revisions/deployed"
						  },
						  "features": {
							"href": "https://api.example.org/v3/apps/second-test-app-guid/features"
						  }
						}
					}
				]
			}`), "Response body matches response:")
			})
		})

		When("no apps can be found", func() {
			BeforeEach(func() {
				appRepo.FetchAppListReturns([]repositories.AppRecord{}, nil)

				router.ServeHTTP(rr, req)
			})

			It("returns status 200 OK", func() {
				Expect(rr.Code).Should(Equal(http.StatusOK), "Matching HTTP response code:")
			})

			It("returns Content-Type as JSON in header", func() {
				contentTypeHeader := rr.Header().Get("Content-Type")
				Expect(contentTypeHeader).Should(Equal(jsonHeader), "Matching Content-Type header:")
			})

			It("returns a CF API formatted Error response", func() {
				Expect(rr.Body.String()).Should(MatchJSON(`{
				"pagination": {
				  "total_results": 0,
				  "total_pages": 1,
				  "first": {
					"href": "https://api.example.org/v3/apps?page=1"
				  },
				  "last": {
					"href": "https://api.example.org/v3/apps?page=1"
				  },
				  "next": null,
				  "previous": null
				},
				"resources": []
			}`), "Response body matches response:")
			})
		})

		When("there is some other error fetching apps", func() {
			BeforeEach(func() {
				appRepo.FetchAppListReturns([]repositories.AppRecord{}, errors.New("unknown!"))

				router.ServeHTTP(rr, req)
			})

			itRespondsWithUnknownError(getRR)
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
