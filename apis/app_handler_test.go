package apis_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/mux"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"

	. "code.cloudfoundry.org/cf-k8s-api/apis"
	"code.cloudfoundry.org/cf-k8s-api/apis/fake"
	"code.cloudfoundry.org/cf-k8s-api/repositories"

	. "github.com/onsi/gomega"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"
	"k8s.io/client-go/rest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	appGUID   = "test-app-guid"
	spaceGUID = "test-space-guid"
)

func TestApp(t *testing.T) {
	spec.Run(t, "the GET /v3/apps/:guid endpoint ", testAppGetHandler, spec.Report(report.Terminal{}))
	spec.Run(t, "the POST /v3/apps endpoint", testAppCreateHandler, spec.Report(report.Terminal{}))
	spec.Run(t, "the GET /v3/apps endpoint", testAppListHandler, spec.Report(report.Terminal{}))
}

func testAppGetHandler(t *testing.T, when spec.G, it spec.S) {
	g := NewWithT(t)

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

	it.Before(func() {
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
		g.Expect(err).NotTo(HaveOccurred())

		rr = httptest.NewRecorder()
		router = mux.NewRouter()
		clientBuilder := new(fake.ClientBuilder)

		apiHandler := &AppHandler{
			ServerURL:   defaultServerURL,
			AppRepo:     appRepo,
			Logger:      logf.Log.WithName(testAppHandlerLoggerName),
			K8sConfig:   &rest.Config{},
			BuildClient: clientBuilder.Spy,
		}
		apiHandler.RegisterRoutes(router)
	})

	when("on the happy path", func() {
		it.Before(func() {
			router.ServeHTTP(rr, req)
		})

		it("returns status 200 OK", func() {
			g.Expect(rr.Code).To(Equal(http.StatusOK), "Matching HTTP response code:")
		})

		it("returns Content-Type as JSON in header", func() {
			contentTypeHeader := rr.Header().Get("Content-Type")
			g.Expect(contentTypeHeader).To(Equal(jsonHeader), "Matching Content-Type header:")
		})

		it("returns the App in the response", func() {
			g.Expect(rr.Body.String()).To(MatchJSON(`{
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

	when("the app cannot be found", func() {
		it.Before(func() {
			appRepo.FetchAppReturns(repositories.AppRecord{}, repositories.NotFoundError{})

			router.ServeHTTP(rr, req)
		})

		// TODO: should we return code 100004 instead?
		itRespondsWithNotFound(it, g, "App not found", getRR)
	})

	when("there is some other error fetching the app", func() {
		it.Before(func() {
			appRepo.FetchAppReturns(repositories.AppRecord{}, errors.New("unknown!"))

			router.ServeHTTP(rr, req)
		})

		itRespondsWithUnknownError(it, g, getRR)
	})
}

func testAppCreateHandler(t *testing.T, when spec.G, it spec.S) {
	g := NewWithT(t)

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
		g.Expect(err).NotTo(HaveOccurred())

		router.ServeHTTP(rr, req)
	}

	it.Before(func() {
		rr = httptest.NewRecorder()
		router = mux.NewRouter()

		appRepo = new(fake.CFAppRepository)
		apiHandler := &AppHandler{
			ServerURL:   defaultServerURL,
			AppRepo:     appRepo,
			Logger:      logf.Log.WithName(testAppHandlerLoggerName),
			K8sConfig:   &rest.Config{},
			BuildClient: new(fake.ClientBuilder).Spy,
		}
		apiHandler.RegisterRoutes(router)
	})

	when("the request body is invalid json", func() {
		it.Before(func() {
			makePostRequest(`{`)
		})

		it("returns a status 400 Bad Request ", func() {
			g.Expect(rr.Code).To(Equal(http.StatusBadRequest), "Matching HTTP response code:")
		})

		it("returns Content-Type as JSON in header", func() {
			contentTypeHeader := rr.Header().Get("Content-Type")
			g.Expect(contentTypeHeader).To(Equal(jsonHeader), "Matching Content-Type header:")
		})

		it("has the expected error response body", func() {
			g.Expect(rr.Body.String()).To(MatchJSON(`{
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

	when("the request body does not validate", func() {
		it.Before(func() {
			makePostRequest(`{"description" : "Invalid Request"}`)
		})

		it("returns a status 422 Bad Request ", func() {
			g.Expect(rr.Code).To(Equal(http.StatusUnprocessableEntity), "Matching HTTP response code:")
		})

		it("returns Content-Type as JSON in header", func() {
			contentTypeHeader := rr.Header().Get("Content-Type")
			g.Expect(contentTypeHeader).To(Equal(jsonHeader), "Matching Content-Type header:")
		})

		it("has the expected error response body", func() {
			g.Expect(rr.Body.String()).To(MatchJSON(`{
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
			g.Expect(rr.Code).To(Equal(http.StatusUnprocessableEntity), "Matching HTTP response code:")
		})

		it("returns Content-Type as JSON in header", func() {
			contentTypeHeader := rr.Header().Get("Content-Type")
			g.Expect(contentTypeHeader).To(Equal(jsonHeader), "Matching Content-Type header:")
		})

		it("has the expected error response body", func() {
			g.Expect(rr.Body.String()).To(MatchJSON(`{
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
			g.Expect(rr.Code).To(Equal(http.StatusUnprocessableEntity), "Matching HTTP response code:")
		})

		it("returns Content-Type as JSON in header", func() {
			contentTypeHeader := rr.Header().Get("Content-Type")
			g.Expect(contentTypeHeader).To(Equal(jsonHeader), "Matching Content-Type header:")
		})

		it("has the expected error response body", func() {
			g.Expect(rr.Body.String()).To(MatchJSON(`{
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
			g.Expect(rr.Code).To(Equal(http.StatusUnprocessableEntity), "Matching HTTP response code:")
		})

		it("returns Content-Type as JSON in header", func() {
			contentTypeHeader := rr.Header().Get("Content-Type")
			g.Expect(contentTypeHeader).To(Equal(jsonHeader), "Matching Content-Type header:")
		})

		it("has the expected error response body", func() {
			g.Expect(rr.Body.String()).To(MatchJSON(`{
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
			g.Expect(rr.Code).To(Equal(http.StatusUnprocessableEntity), "Matching HTTP response code:")
		})

		it("returns Content-Type as JSON in header", func() {
			contentTypeHeader := rr.Header().Get("Content-Type")
			g.Expect(contentTypeHeader).To(Equal(jsonHeader), "Matching Content-Type header:")
		})

		it("has the expected error response body", func() {
			decoder := json.NewDecoder(rr.Body)
			decoder.DisallowUnknownFields()

			body := struct {
				Errors []struct {
					Title  string `json:"title"`
					Code   int    `json:"code"`
					Detail string `json:"detail"`
				} `json:"errors"`
			}{}
			g.Expect(decoder.Decode(&body)).To(Succeed())

			g.Expect(body.Errors).To(HaveLen(1))
			g.Expect(body.Errors[0].Title).To(Equal("CF-UnprocessableEntity"))
			g.Expect(body.Errors[0].Code).To(Equal(10008))
			g.Expect(body.Errors[0].Detail).NotTo(BeEmpty())
			subDetails := strings.Split(body.Errors[0].Detail, ",")
			g.Expect(subDetails).To(ConsistOf(
				"Type is a required field",
				"Buildpacks is a required field",
				"Stack is a required field",
			))
		})
	})

	when("the space does not exist", func() {
		it.Before(func() {
			appRepo.FetchNamespaceReturns(repositories.SpaceRecord{},
				repositories.PermissionDeniedOrNotFoundError{Err: errors.New("not found")})

			requestBody := initializeCreateAppRequestBody(testAppName, "no-such-guid", nil, nil, nil)
			makePostRequest(requestBody)
		})

		it("returns a status 422 Unprocessable Entity", func() {
			g.Expect(rr.Code).To(Equal(http.StatusUnprocessableEntity), "Matching HTTP response code:")
		})

		it("returns Content-Type as JSON in header", func() {
			contentTypeHeader := rr.Header().Get("Content-Type")
			g.Expect(contentTypeHeader).To(Equal(jsonHeader), "Matching Content-Type header:")
		})

		it("returns a CF API formatted Error response", func() {
			g.Expect(rr.Body.String()).To(MatchJSON(`{
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

	when("the app already exists, but AppCreate returns false due to validating webhook rejection", func() {
		it.Before(func() {
			controllerError := new(k8serrors.StatusError)
			controllerError.ErrStatus.Reason = "CFApp with the same spec.name exists"
			appRepo.CreateAppReturns(repositories.AppRecord{}, controllerError)

			requestBody := initializeCreateAppRequestBody(testAppName, spaceGUID, nil, nil, nil)
			makePostRequest(requestBody)
		})

		it("returns a status 422 Unprocessable Entity", func() {
			g.Expect(rr.Code).To(Equal(http.StatusUnprocessableEntity), "Matching HTTP response code:")
		})

		it("returns Content-Type as JSON in header", func() {
			contentTypeHeader := rr.Header().Get("Content-Type")
			g.Expect(contentTypeHeader).To(Equal(jsonHeader), "Matching Content-Type header:")
		})

		it("returns a CF API formatted Error response", func() {
			g.Expect(rr.Body.String()).To(MatchJSON(`{
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

	when("the app already exists, but CreateApp returns false due to a non webhook k8s error", func() {
		it.Before(func() {
			controllerError := new(k8serrors.StatusError)
			controllerError.ErrStatus.Reason = "different k8s api error"
			appRepo.CreateAppReturns(repositories.AppRecord{}, controllerError)

			requestBody := initializeCreateAppRequestBody(testAppName, spaceGUID, nil, nil, nil)
			makePostRequest(requestBody)
		})

		itRespondsWithUnknownError(it, g, getRR)
	})

	when("the namespace exists and app does not exist and", func() {
		when("a plain POST test app request is sent without env vars or metadata", func() {
			it.Before(func() {
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

			it("should invoke repo CreateApp with a random GUID", func() {
				g.Expect(appRepo.CreateAppCallCount()).To(Equal(1), "Repo CreateApp count was not invoked 1 time")
				_, _, createAppRecord := appRepo.CreateAppArgsForCall(0)
				g.Expect(createAppRecord.GUID).To(MatchRegexp("^[-0-9a-f]{36}$"), "CreateApp record GUID was not a 36 character guid")
			})

			it("should not invoke repo CreateAppEnvironmentVariables when no environment variables are provided", func() {
				g.Expect(appRepo.CreateAppEnvironmentVariablesCallCount()).To(BeZero(), "Repo CreateAppEnvironmentVariables was invoked even though no environment vars were provided")
			})

			it("return status 200 OK", func() {
				g.Expect(rr.Code).To(Equal(http.StatusOK), "Matching HTTP response code:")
			})

			it("returns Content-Type as JSON in header", func() {
				contentTypeHeader := rr.Header().Get("Content-Type")
				g.Expect(contentTypeHeader).To(Equal(jsonHeader), "Matching Content-Type header:")
			})

			it("returns the \"created app\"(the mock response record) in the response", func() {
				g.Expect(rr.Body.String()).To(MatchJSON(`{
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

		when("a POST test app request is sent with env vars and", func() {
			var (
				testEnvironmentVariables map[string]string
				requestBody              string
			)

			it.Before(func() {
				testEnvironmentVariables = map[string]string{"foo": "foo", "bar": "bar"}

				requestBody = initializeCreateAppRequestBody(testAppName, spaceGUID, testEnvironmentVariables, nil, nil)
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
					g.Expect(appRepo.CreateAppEnvironmentVariablesCallCount()).To(Equal(1), "Repo CreateAppEnvironmentVariables count was not invoked 1 time")
					_, _, createAppEnvVarsRecord := appRepo.CreateAppEnvironmentVariablesArgsForCall(0)
					g.Expect(createAppEnvVarsRecord.EnvironmentVariables).To(Equal(testEnvironmentVariables))
					g.Expect(createAppEnvVarsRecord.SpaceGUID).To(Equal(spaceGUID))
				})

				it("should call Repo CreateApp and provide the name of the created env Secret", func() {
					g.Expect(appRepo.CreateAppCallCount()).To(Equal(1), "Repo CreateApp count was not invoked 1 time")
					_, _, createAppRecord := appRepo.CreateAppArgsForCall(0)
					g.Expect(createAppRecord.EnvSecretName).To(Equal(createEnvVarsResponseName))
				})
			})

			when("there will be a repository error with creating the env vars", func() {
				it.Before(func() {
					appRepo.CreateAppEnvironmentVariablesReturns(repositories.AppEnvVarsRecord{}, errors.New("intentional error"))

					makePostRequest(requestBody)
				})

				itRespondsWithUnknownError(it, g, getRR)
			})
		})

		when("a POST test app request is sent with metadata labels", func() {
			var (
				testLabels map[string]string
			)

			it.Before(func() {
				testLabels = map[string]string{"foo": "foo", "bar": "bar"}

				requestBody := initializeCreateAppRequestBody(testAppName, spaceGUID, nil, testLabels, nil)
				makePostRequest(requestBody)
			})

			it("should pass along the labels to CreateApp", func() {
				g.Expect(appRepo.CreateAppCallCount()).To(Equal(1), "Repo CreateApp count was not invoked 1 time")
				_, _, createAppRecord := appRepo.CreateAppArgsForCall(0)
				g.Expect(createAppRecord.Labels).To(Equal(testLabels))
			})
		})

		when("a POST test app request is sent with metadata annotations", func() {
			var (
				testAnnotations map[string]string
			)

			it.Before(func() {
				testAnnotations = map[string]string{"foo": "foo", "bar": "bar"}
				requestBody := initializeCreateAppRequestBody(testAppName, spaceGUID, nil, nil, testAnnotations)
				makePostRequest(requestBody)
			})

			it("should pass along the annotations to CreateApp", func() {
				g.Expect(appRepo.CreateAppCallCount()).To(Equal(1), "Repo CreateApp count was not invoked 1 time")
				_, _, createAppRecord := appRepo.CreateAppArgsForCall(0)
				g.Expect(createAppRecord.Annotations).To(Equal(testAnnotations))
			})
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

func testAppListHandler(t *testing.T, when spec.G, it spec.S) {
	g := NewWithT(t)

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

	it.Before(func() {
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
		g.Expect(err).NotTo(HaveOccurred())

		rr = httptest.NewRecorder()
		router = mux.NewRouter()
		clientBuilder := new(fake.ClientBuilder)

		apiHandler := &AppHandler{
			ServerURL:   defaultServerURL,
			AppRepo:     appRepo,
			Logger:      logf.Log.WithName(testAppHandlerLoggerName),
			K8sConfig:   &rest.Config{},
			BuildClient: clientBuilder.Spy,
		}
		apiHandler.RegisterRoutes(router)
	})

	when("on the happy path", func() {
		it.Before(func() {
			router.ServeHTTP(rr, req)
		})

		it("returns status 200 OK", func() {
			g.Expect(rr.Code).Should(Equal(http.StatusOK), "Matching HTTP response code:")
		})

		it("returns Content-Type as JSON in header", func() {
			contentTypeHeader := rr.Header().Get("Content-Type")
			g.Expect(contentTypeHeader).Should(Equal(jsonHeader), "Matching Content-Type header:")
		})

		it("returns the Pagination Data and App Resources in the response", func() {
			g.Expect(rr.Body.String()).Should(MatchJSON(`{
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

	when("no apps can be found", func() {
		it.Before(func() {
			appRepo.FetchAppListReturns([]repositories.AppRecord{}, nil)

			router.ServeHTTP(rr, req)
		})

		it("returns status 200 OK", func() {
			g.Expect(rr.Code).Should(Equal(http.StatusOK), "Matching HTTP response code:")
		})

		it("returns Content-Type as JSON in header", func() {
			contentTypeHeader := rr.Header().Get("Content-Type")
			g.Expect(contentTypeHeader).Should(Equal(jsonHeader), "Matching Content-Type header:")
		})

		it("returns a CF API formatted Error response", func() {
			g.Expect(rr.Body.String()).Should(MatchJSON(`{
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

	when("there is some other error fetching apps", func() {
		it.Before(func() {
			appRepo.FetchAppListReturns([]repositories.AppRecord{}, errors.New("unknown!"))

			router.ServeHTTP(rr, req)
		})

		itRespondsWithUnknownError(it, g, getRR)
	})
}
