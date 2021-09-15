package apis_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"code.cloudfoundry.org/cf-k8s-api/apis/fake"

	logf "sigs.k8s.io/controller-runtime/pkg/log"

	"code.cloudfoundry.org/cf-k8s-api/apis"
	"code.cloudfoundry.org/cf-k8s-api/presenter"
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
		rr                  *httptest.ResponseRecorder
		FetchAppResponseApp repositories.AppRecord
		FetchAppErr         error
	)

	when("the GET /v3/apps/:guid  endpoint returns successfully", func() {
		it.Before(func() {
			FetchAppResponseApp = repositories.AppRecord{
				GUID:      "test-app-guid",
				Name:      "test-app",
				SpaceGUID: "test-space-guid",
				State:     repositories.DesiredState("STOPPED"),
				Lifecycle: repositories.Lifecycle{
					Data: repositories.LifecycleData{
						Buildpacks: []string{},
						Stack:      "",
					},
				},
			}
			FetchAppErr = nil

			fakeAppRepo := &fake.CFAppRepository{}
			fakeAppRepo.FetchAppReturns(FetchAppResponseApp, FetchAppErr)

			req, err := http.NewRequest("GET", "/v3/apps/my-app-guid", nil)
			Expect(err).NotTo(HaveOccurred())

			rr = httptest.NewRecorder()
			clientBuilder := new(fake.ClientBuilder)

			apiHandler := apis.AppHandler{
				ServerURL:   defaultServerURL,
				AppRepo:     fakeAppRepo,
				Logger:      logf.Log.WithName(testAppHandlerLoggerName),
				K8sConfig:   &rest.Config{},
				BuildClient: clientBuilder.Spy,
			}

			handler := http.HandlerFunc(apiHandler.AppGetHandler)

			handler.ServeHTTP(rr, req)
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
			expectedBody, err := json.Marshal(presenter.AppResponse{
				Name:  "test-app",
				GUID:  "test-app-guid",
				State: "STOPPED",
				Relationships: presenter.Relationships{
					"space": presenter.Relationship{
						Data: presenter.RelationshipData{
							GUID: "test-space-guid",
						},
					},
				},
				Lifecycle: presenter.Lifecycle{Data: presenter.LifecycleData{
					Buildpacks: []string{},
					Stack:      "",
				}},
				Metadata: presenter.Metadata{
					Labels:      map[string]string{},
					Annotations: map[string]string{},
				},
				Links: presenter.AppLinks{
					Self: presenter.Link{
						HREF: "https://api.example.org/v3/apps/test-app-guid",
					},
					Space: presenter.Link{
						HREF: "https://api.example.org/v3/spaces/test-space-guid",
					},
					Processes: presenter.Link{
						HREF: "https://api.example.org/v3/apps/test-app-guid/processes",
					},
					Packages: presenter.Link{
						HREF: "https://api.example.org/v3/apps/test-app-guid/packages",
					},
					EnvironmentVariables: presenter.Link{
						HREF: "https://api.example.org/v3/apps/test-app-guid/environment_variables",
					},
					CurrentDroplet: presenter.Link{
						HREF: "https://api.example.org/v3/apps/test-app-guid/droplets/current",
					},
					Droplets: presenter.Link{
						HREF: "https://api.example.org/v3/apps/test-app-guid/droplets",
					},
					Tasks: presenter.Link{},
					StartAction: presenter.Link{
						HREF:   "https://api.example.org/v3/apps/test-app-guid/actions/start",
						Method: "POST",
					},
					StopAction: presenter.Link{
						HREF:   "https://api.example.org/v3/apps/test-app-guid/actions/stop",
						Method: "POST",
					},
					Revisions:         presenter.Link{},
					DeployedRevisions: presenter.Link{},
					Features:          presenter.Link{},
				},
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(rr.Body.String()).Should(MatchJSON(expectedBody), "Response body matches response:")
		})
	})

	when("the app cannot be found", func() {
		it.Before(func() {
			FetchAppResponseApp = repositories.AppRecord{}
			FetchAppErr = repositories.NotFoundError{Err: errors.New("not found")}

			req, err := http.NewRequest("GET", "/v3/apps/my-app-guid", nil)
			Expect(err).NotTo(HaveOccurred())

			fakeAppRepo := &fake.CFAppRepository{}
			fakeAppRepo.FetchAppReturns(FetchAppResponseApp, FetchAppErr)

			rr = httptest.NewRecorder()
			clientBuilder := new(fake.ClientBuilder)

			apiHandler := apis.AppHandler{
				ServerURL:   defaultServerURL,
				AppRepo:     fakeAppRepo,
				Logger:      logf.Log.WithName(testAppHandlerLoggerName),
				K8sConfig:   &rest.Config{},
				BuildClient: clientBuilder.Spy,
			}

			handler := http.HandlerFunc(apiHandler.AppGetHandler)

			handler.ServeHTTP(rr, req)
		})

		it("returns a CF API formatted Error response", func() {
			expectedBody, err := json.Marshal(presenter.ErrorsResponse{Errors: []presenter.PresentedError{{
				Title:  "App not found",
				Detail: "CF-ResourceNotFound",
				Code:   10010,
			}}})

			httpStatus := rr.Code
			Expect(httpStatus).Should(Equal(http.StatusNotFound), "Matching HTTP response code:")

			Expect(err).NotTo(HaveOccurred())
			Expect(rr.Body.String()).Should(MatchJSON(expectedBody), "Response body matches response:")
		})
	})

	when("there is some other error fetching the app", func() {
		it.Before(func() {
			FetchAppResponseApp = repositories.AppRecord{}
			FetchAppErr = errors.New("unknown!")

			req, err := http.NewRequest("GET", "/v3/apps/my-app-guid", nil)
			Expect(err).NotTo(HaveOccurred())

			fakeAppRepo := &fake.CFAppRepository{}
			fakeAppRepo.FetchAppReturns(FetchAppResponseApp, FetchAppErr)

			rr = httptest.NewRecorder()
			clientBuilder := new(fake.ClientBuilder)

			apiHandler := apis.AppHandler{
				ServerURL:   defaultServerURL,
				AppRepo:     fakeAppRepo,
				Logger:      logf.Log.WithName(testAppHandlerLoggerName),
				K8sConfig:   &rest.Config{},
				BuildClient: clientBuilder.Spy,
			}

			handler := http.HandlerFunc(apiHandler.AppGetHandler)

			handler.ServeHTTP(rr, req)
		})

		it("returns a CF API formatted Error response", func() {
			expectedBody, err := json.Marshal(presenter.ErrorsResponse{Errors: []presenter.PresentedError{{
				Title:  "UnknownError",
				Detail: "An unknown error occurred.",
				Code:   10001,
			}}})

			httpStatus := rr.Code
			Expect(httpStatus).Should(Equal(http.StatusInternalServerError), "Matching HTTP response code:")

			Expect(err).NotTo(HaveOccurred())
			Expect(rr.Body.String()).Should(MatchJSON(expectedBody), "Response body matches response:")
		})
	})

}

func initializeCreateAppRequestBody(appName, spaceGUID string, envVars, labels, annotations map[string]string) []byte {
	marshaledEnvironmentVariables, _ := json.Marshal(envVars)
	marshaledLabels, _ := json.Marshal(labels)
	marshaledAnnotations, _ := json.Marshal(annotations)

	return []byte(`{
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
					}`)
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
		rr *httptest.ResponseRecorder
	)

	when("the POST /v3/apps endpoint is invoked and", func() {

		when("the request body is invalid", func() {
			it.Before(func() {
				requestBody := []byte(`{"description" : "Invalid Request"}`)

				req, err := http.NewRequest("POST", "/v3/apps", bytes.NewReader(requestBody))
				Expect(err).NotTo(HaveOccurred())

				apiHandler := apis.AppHandler{
					ServerURL: defaultServerURL,
					AppRepo:   &fake.CFAppRepository{},
					Logger:    logf.Log.WithName(testAppHandlerLoggerName),
					K8sConfig: &rest.Config{},
				}

				handler := http.HandlerFunc(apiHandler.AppCreateHandler)
				rr = httptest.NewRecorder()
				handler.ServeHTTP(rr, req)

			})
			it("returns a status 400 Bad Request ", func() {
				Expect(rr.Code).To(Equal(http.StatusBadRequest))
			})
			it("has the expected error response body", func() {
				expectedBody, err := json.Marshal(presenter.ErrorsResponse{Errors: []presenter.PresentedError{{
					Title:  "CF-MessageParseError",
					Detail: "Request invalid due to parse error: invalid request body",
					Code:   1001,
				}}})
				Expect(err).NotTo(HaveOccurred())
				Expect(rr.Body).To(MatchJSON(expectedBody))
			})

		})

		when("the request body is invalid with invalid app name", func() {
			it.Before(func() {
				requestBody := `{
									"name": 12345,
									"relationships": {
									  "space": {
										"data": {
										  "guid": "2f35885d-0c9d-4423-83ad-fd05066f8576"
										}
									  }
									}
								  }`

				req, err := http.NewRequest("POST", "/v3/apps", strings.NewReader(requestBody))
				Expect(err).NotTo(HaveOccurred())

				apiHandler := apis.AppHandler{
					ServerURL: defaultServerURL,
					AppRepo:   &fake.CFAppRepository{},
					Logger:    logf.Log.WithName(testAppHandlerLoggerName),
					K8sConfig: &rest.Config{},
				}

				rr = httptest.NewRecorder()
				handler := http.HandlerFunc(apiHandler.AppCreateHandler)
				handler.ServeHTTP(rr, req)

			})
			it("returns a status 422 Unprocessable Entity", func() {
				Expect(rr.Code).To(Equal(http.StatusUnprocessableEntity))
			})
			it("has the expected error response body", func() {
				Expect(rr.Body).To(MatchJSON(`{
                  "errors": [
                    {
				      "title":  "CF-UnprocessableEntity",
				   	  "detail": "Name must be a string",
				 	  "code":   10008
                    }
                  ]
				}`))
			})

		})

		when("the request body is invalid with invalid environment variable object", func() {
			it.Before(func() {
				requestBody := []byte(`{
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

				req, err := http.NewRequest("POST", "/v3/apps", bytes.NewReader(requestBody))
				Expect(err).NotTo(HaveOccurred())

				apiHandler := apis.AppHandler{
					ServerURL: defaultServerURL,
					AppRepo:   &fake.CFAppRepository{},
					Logger:    logf.Log.WithName(testAppHandlerLoggerName),
					K8sConfig: &rest.Config{},
				}

				rr = httptest.NewRecorder()
				handler := http.HandlerFunc(apiHandler.AppCreateHandler)
				handler.ServeHTTP(rr, req)

			})
			it("returns a status 422 Unprocessable Entity", func() {
				Expect(rr.Code).To(Equal(http.StatusUnprocessableEntity))
			})
			it("has the expected error response body", func() {
				expectedBody, err := json.Marshal(presenter.ErrorsResponse{Errors: []presenter.PresentedError{{
					Title:  "CF-UnprocessableEntity",
					Detail: "Environment_variables must be a map[string]string",
					Code:   10008,
				}}})
				Expect(err).NotTo(HaveOccurred())
				Expect(rr.Body).To(MatchJSON(expectedBody))
			})

		})

		when("the request body is invalid with missing required name field", func() {
			it.Before(func() {
				requestBody := []byte(`{
										"relationships": {
										  "space": {
											"data": {
											  "guid": "0c78dd5d-c723-4f2e-b168-df3c3e1d0806"
											}
										  }
										}
									  }`)

				req, err := http.NewRequest("POST", "/v3/apps", bytes.NewReader(requestBody))
				Expect(err).NotTo(HaveOccurred())

				apiHandler := apis.AppHandler{
					ServerURL: defaultServerURL,
					AppRepo:   &fake.CFAppRepository{},
					Logger:    logf.Log.WithName(testAppHandlerLoggerName),
					K8sConfig: &rest.Config{},
				}

				rr = httptest.NewRecorder()
				handler := http.HandlerFunc(apiHandler.AppCreateHandler)
				handler.ServeHTTP(rr, req)

			})
			it("returns a status 422 Unprocessable Entity", func() {
				Expect(rr.Code).To(Equal(http.StatusUnprocessableEntity))
			})
			it("has the expected error response body", func() {
				expectedBody, err := json.Marshal(presenter.ErrorsResponse{Errors: []presenter.PresentedError{{
					Title:  "CF-UnprocessableEntity",
					Detail: "Name must be a string",
					Code:   10008,
				}}})
				Expect(err).NotTo(HaveOccurred())
				Expect(rr.Body).To(MatchJSON(expectedBody))
			})

		})

		when("the request body is invalid with missing data within lifecycle", func() {
			it.Before(func() {
				requestBody := []byte(`{
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

				req, err := http.NewRequest("POST", "/v3/apps", bytes.NewReader(requestBody))
				Expect(err).NotTo(HaveOccurred())

				apiHandler := apis.AppHandler{
					ServerURL: defaultServerURL,
					AppRepo:   &fake.CFAppRepository{},
					Logger:    logf.Log.WithName(testAppHandlerLoggerName),
					K8sConfig: &rest.Config{},
				}

				rr = httptest.NewRecorder()
				handler := http.HandlerFunc(apiHandler.AppCreateHandler)
				handler.ServeHTTP(rr, req)

			})
			it("returns a status 422 Unprocessable Entity", func() {
				Expect(rr.Code).To(Equal(http.StatusUnprocessableEntity))
			})
			it("has the expected error response body", func() {
				expectedBody, err := json.Marshal(presenter.ErrorsResponse{Errors: []presenter.PresentedError{{
					Title:  "CF-UnprocessableEntity",
					Detail: "Type must be a string,Buildpacks must be a []string,Stack must be a string",
					Code:   10008,
				}}})
				Expect(err).NotTo(HaveOccurred())
				Expect(rr.Body).To(MatchJSON(expectedBody))
			})

		})

		when("the space does not exist", func() {
			it.Before(func() {
				nonExistingSpaceGUID := "0c78dd5d-c723-4f2e-b168-df3c3e1d0806"
				requestBody := initializeCreateAppRequestBody(testAppName, nonExistingSpaceGUID, nil, nil, nil)

				req, err := http.NewRequest("POST", "/v3/apps", bytes.NewReader(requestBody))
				Expect(err).NotTo(HaveOccurred())

				fakeAppRepo := &fake.CFAppRepository{}
				fetchNamespaceResponse := repositories.SpaceRecord{}
				fetchNamespaceErr := repositories.PermissionDeniedOrNotFoundError{Err: errors.New("not found")}
				fakeAppRepo.FetchNamespaceReturns(fetchNamespaceResponse, fetchNamespaceErr)

				rr = httptest.NewRecorder()
				clientBuilder := new(fake.ClientBuilder)

				apiHandler := apis.AppHandler{
					ServerURL:   defaultServerURL,
					AppRepo:     fakeAppRepo,
					Logger:      logf.Log.WithName(testAppHandlerLoggerName),
					K8sConfig:   &rest.Config{},
					BuildClient: clientBuilder.Spy,
				}

				handler := http.HandlerFunc(apiHandler.AppCreateHandler)
				handler.ServeHTTP(rr, req)
			})

			it("returns a CF API formatted Error response", func() {
				expectedBody, err := json.Marshal(presenter.ErrorsResponse{Errors: []presenter.PresentedError{{
					Detail: "Invalid space. Ensure that the space exists and you have access to it.",
					Title:  "CF-UnprocessableEntity",
					Code:   10008,
				}}})

				httpStatus := rr.Code
				Expect(httpStatus).Should(Equal(http.StatusUnprocessableEntity), "Matching HTTP response code:")

				Expect(err).NotTo(HaveOccurred())
				Expect(rr.Body.String()).Should(MatchJSON(expectedBody), "Response body matches response:")
			})
		})

		when("the app already exists", func() {
			it.Before(func() {
				requestBody := initializeCreateAppRequestBody(testAppName, testSpaceGUID, nil, nil, nil)

				req, err := http.NewRequest("POST", "/v3/apps", bytes.NewReader(requestBody))
				Expect(err).NotTo(HaveOccurred())

				fakeAppRepo := &fake.CFAppRepository{}
				fakeAppRepo.AppExistsReturns(true, nil)

				rr = httptest.NewRecorder()
				clientBuilder := new(fake.ClientBuilder)

				apiHandler := apis.AppHandler{
					ServerURL:   defaultServerURL,
					AppRepo:     fakeAppRepo,
					Logger:      logf.Log.WithName(testAppHandlerLoggerName),
					K8sConfig:   &rest.Config{},
					BuildClient: clientBuilder.Spy,
				}
				handler := http.HandlerFunc(apiHandler.AppCreateHandler)
				handler.ServeHTTP(rr, req)
			})

			it("returns a CF API formatted Error response", func() {
				expectedBody, err := json.Marshal(presenter.ErrorsResponse{Errors: []presenter.PresentedError{{
					Detail: "App with the name 'test-app' already exists.",
					Title:  "CF-UniquenessError",
					Code:   10016,
				}}})

				httpStatus := rr.Code
				Expect(httpStatus).Should(Equal(http.StatusUnprocessableEntity), "Matching HTTP response code:")

				Expect(err).NotTo(HaveOccurred())
				Expect(rr.Body.String()).Should(MatchJSON(expectedBody), "Response body matches response:")
			})
		})

		when("the namespace exists and app does not exist and", func() {
			var fakeAppRepo *fake.CFAppRepository

			it.Before(func() {
				fakeAppRepo = &fake.CFAppRepository{}
				fakeAppRepo.AppExistsReturns(false, nil)
			})

			when("a plain POST test app request is sent without env vars or metadata", func() {

				const testAppGUID = "test-app-guid"

				it.Before(func() {
					requestBody := initializeCreateAppRequestBody(testAppName, testSpaceGUID, nil, nil, nil)

					CreateAppResponse := repositories.AppRecord{
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
					}
					fakeAppRepo.CreateAppReturns(CreateAppResponse, nil)

					req, err := http.NewRequest("POST", "/v3/apps", bytes.NewReader(requestBody))
					Expect(err).NotTo(HaveOccurred())

					rr = httptest.NewRecorder()
					clientBuilder := new(fake.ClientBuilder)

					apiHandler := apis.AppHandler{
						ServerURL:   defaultServerURL,
						AppRepo:     fakeAppRepo,
						Logger:      logf.Log.WithName(testAppHandlerLoggerName),
						K8sConfig:   &rest.Config{},
						BuildClient: clientBuilder.Spy,
					}
					handler := http.HandlerFunc(apiHandler.AppCreateHandler)
					handler.ServeHTTP(rr, req)
				})

				it("should invoke repo CreateApp with a random GUID", func() {
					Expect(fakeAppRepo.CreateAppCallCount()).To(Equal(1), "Repo CreateApp count was not invoked 1 time")
					_, _, createAppRecord := fakeAppRepo.CreateAppArgsForCall(0)
					Expect(createAppRecord.GUID).To(MatchRegexp("^[-0-9a-f]{36}$"), "CreateApp record GUID was not a 36 character guid")
				})

				it("should not invoke repo CreateAppEnvironmentVariables when no environment variables are provided", func() {
					Expect(fakeAppRepo.CreateAppEnvironmentVariablesCallCount()).To(BeZero(), "Repo CreateAppEnvironmentVariables was invoked even though no environment vars were provided")
				})

				it("return status 200OK", func() {
					httpStatus := rr.Code
					Expect(httpStatus).Should(Equal(http.StatusOK), "Matching HTTP response code:")
				})

				it("returns Content-Type as JSON in header", func() {
					contentTypeHeader := rr.Header().Get("Content-Type")
					Expect(contentTypeHeader).Should(Equal(jsonHeader), "Matching Content-Type header:")
				})

				it("returns the \"created app\"(the mock response record) in the response", func() {
					expectedBody, err := json.Marshal(presenter.AppResponse{
						Name:  testAppName,
						GUID:  testAppGUID,
						State: "STOPPED",
						Relationships: presenter.Relationships{
							"space": presenter.Relationship{
								Data: presenter.RelationshipData{
									GUID: testSpaceGUID,
								},
							},
						},
						Lifecycle: presenter.Lifecycle{Data: presenter.LifecycleData{
							Buildpacks: []string{},
							Stack:      "",
						}},
						Metadata: presenter.Metadata{
							Labels:      map[string]string{},
							Annotations: map[string]string{},
						},
						Links: presenter.AppLinks{
							Self: presenter.Link{
								HREF: "https://api.example.org/v3/apps/" + testAppGUID,
							},
							Space: presenter.Link{
								HREF: "https://api.example.org/v3/spaces/" + testSpaceGUID,
							},
							Processes: presenter.Link{
								HREF: "https://api.example.org/v3/apps/" + testAppGUID + "/processes",
							},
							Packages: presenter.Link{
								HREF: "https://api.example.org/v3/apps/" + testAppGUID + "/packages",
							},
							EnvironmentVariables: presenter.Link{
								HREF: "https://api.example.org/v3/apps/" + testAppGUID + "/environment_variables",
							},
							CurrentDroplet: presenter.Link{
								HREF: "https://api.example.org/v3/apps/" + testAppGUID + "/droplets/current",
							},
							Droplets: presenter.Link{
								HREF: "https://api.example.org/v3/apps/" + testAppGUID + "/droplets",
							},
							Tasks: presenter.Link{},
							StartAction: presenter.Link{
								HREF:   "https://api.example.org/v3/apps/" + testAppGUID + "/actions/start",
								Method: "POST",
							},
							StopAction: presenter.Link{
								HREF:   "https://api.example.org/v3/apps/" + testAppGUID + "/actions/stop",
								Method: "POST",
							},
							Revisions:         presenter.Link{},
							DeployedRevisions: presenter.Link{},
							Features:          presenter.Link{},
						},
					})

					Expect(rr.Body.String()).Should(MatchJSON(expectedBody), "Response body matches response:")
					Expect(err).NotTo(HaveOccurred())
				})
			})

			when("a POST test app request is sent with env vars and", func() {
				var (
					testEnvironmentVariables map[string]string
					req                      *http.Request
				)

				it.Before(func() {
					testEnvironmentVariables = map[string]string{"foo": "foo", "bar": "bar"}
					requestBody := initializeCreateAppRequestBody(testAppName, testSpaceGUID, testEnvironmentVariables, nil, nil)
					var err error
					req, err = http.NewRequest("POST", "/v3/apps", bytes.NewReader(requestBody))
					Expect(err).NotTo(HaveOccurred())
				})

				when("the env var repository is working and will not return an error", func() {
					const createEnvVarsResponseName = "testAppGUID-env"

					it.Before(func() {
						CreateEnvVarsResponse := repositories.AppEnvVarsRecord{
							Name: createEnvVarsResponseName,
						}
						fakeAppRepo.CreateAppEnvironmentVariablesReturns(CreateEnvVarsResponse, nil)

						rr = httptest.NewRecorder()
						clientBuilder := new(fake.ClientBuilder)

						apiHandler := apis.AppHandler{
							ServerURL:   defaultServerURL,
							AppRepo:     fakeAppRepo,
							Logger:      logf.Log.WithName(testAppHandlerLoggerName),
							K8sConfig:   &rest.Config{},
							BuildClient: clientBuilder.Spy,
						}
						handler := http.HandlerFunc(apiHandler.AppCreateHandler)
						handler.ServeHTTP(rr, req)
					})

					it("should call Repo CreateAppEnvironmentVariables with the space and environment vars", func() {
						Expect(fakeAppRepo.CreateAppEnvironmentVariablesCallCount()).To(Equal(1), "Repo CreateAppEnvironmentVariables count was not invoked 1 time")
						_, _, createAppEnvVarsRecord := fakeAppRepo.CreateAppEnvironmentVariablesArgsForCall(0)
						Expect(createAppEnvVarsRecord.EnvironmentVariables).To(Equal(testEnvironmentVariables))
						Expect(createAppEnvVarsRecord.SpaceGUID).To(Equal(testSpaceGUID))
					})

					it("should call Repo CreateApp and provide the name of the created env Secret", func() {
						Expect(fakeAppRepo.CreateAppCallCount()).To(Equal(1), "Repo CreateApp count was not invoked 1 time")
						_, _, createAppRecord := fakeAppRepo.CreateAppArgsForCall(0)
						Expect(createAppRecord.EnvSecretName).To(Equal(createEnvVarsResponseName))
					})
				})

				when("there will be a repository error with creating the env vars", func() {
					it.Before(func() {
						fakeAppRepo.CreateAppEnvironmentVariablesReturns(repositories.AppEnvVarsRecord{}, errors.New("intentional error"))
						rr = httptest.NewRecorder()
						clientBuilder := new(fake.ClientBuilder)

						apiHandler := apis.AppHandler{
							ServerURL:   defaultServerURL,
							AppRepo:     fakeAppRepo,
							Logger:      logf.Log.WithName(testAppHandlerLoggerName),
							K8sConfig:   &rest.Config{},
							BuildClient: clientBuilder.Spy,
						}
						handler := http.HandlerFunc(apiHandler.AppCreateHandler)
						handler.ServeHTTP(rr, req)
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
					req, err := http.NewRequest("POST", "/v3/apps", bytes.NewReader(requestBody))
					Expect(err).NotTo(HaveOccurred())

					rr = httptest.NewRecorder()
					clientBuilder := new(fake.ClientBuilder)

					apiHandler := apis.AppHandler{
						ServerURL:   defaultServerURL,
						AppRepo:     fakeAppRepo,
						Logger:      logf.Log.WithName(testAppHandlerLoggerName),
						K8sConfig:   &rest.Config{},
						BuildClient: clientBuilder.Spy,
					}
					handler := http.HandlerFunc(apiHandler.AppCreateHandler)
					handler.ServeHTTP(rr, req)
				})
				it("should pass along the labels to CreateApp", func() {
					Expect(fakeAppRepo.CreateAppCallCount()).To(Equal(1), "Repo CreateApp count was not invoked 1 time")
					_, _, createAppRecord := fakeAppRepo.CreateAppArgsForCall(0)
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
					req, err := http.NewRequest("POST", "/v3/apps", bytes.NewReader(requestBody))
					Expect(err).NotTo(HaveOccurred())

					rr = httptest.NewRecorder()
					clientBuilder := new(fake.ClientBuilder)

					apiHandler := apis.AppHandler{
						ServerURL:   defaultServerURL,
						AppRepo:     fakeAppRepo,
						Logger:      logf.Log.WithName(testAppHandlerLoggerName),
						K8sConfig:   &rest.Config{},
						BuildClient: clientBuilder.Spy,
					}

					handler := http.HandlerFunc(apiHandler.AppCreateHandler)
					handler.ServeHTTP(rr, req)
				})
				it("should pass along the annotations to CreateApp", func() {
					Expect(fakeAppRepo.CreateAppCallCount()).To(Equal(1), "Repo CreateApp count was not invoked 1 time")
					_, _, createAppRecord := fakeAppRepo.CreateAppArgsForCall(0)
					Expect(createAppRecord.Annotations).To(Equal(testAnnotations))
				})
			})

		})

	})
}
