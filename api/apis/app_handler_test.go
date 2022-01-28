package apis_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	. "code.cloudfoundry.org/cf-k8s-controllers/api/apis"
	"code.cloudfoundry.org/cf-k8s-controllers/api/apis/fake"
	"code.cloudfoundry.org/cf-k8s-controllers/api/authorization"
	"code.cloudfoundry.org/cf-k8s-controllers/api/repositories"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
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
		appRepo             *fake.CFAppRepository
		dropletRepo         *fake.CFDropletRepository
		processRepo         *fake.CFProcessRepository
		routeRepo           *fake.CFRouteRepository
		scaleAppProcessFunc *fake.ScaleAppProcess
		domainRepo          *fake.CFDomainRepository
		podRepo             *fake.PodRepository
		req                 *http.Request
	)

	BeforeEach(func() {
		appRepo = new(fake.CFAppRepository)
		dropletRepo = new(fake.CFDropletRepository)
		processRepo = new(fake.CFProcessRepository)
		routeRepo = new(fake.CFRouteRepository)
		domainRepo = new(fake.CFDomainRepository)
		podRepo = new(fake.PodRepository)
		scaleAppProcessFunc = new(fake.ScaleAppProcess)

		apiHandler := NewAppHandler(
			logf.Log.WithName(testAppHandlerLoggerName),
			*serverURL,
			appRepo,
			dropletRepo,
			processRepo,
			routeRepo,
			domainRepo,
			podRepo,
			scaleAppProcessFunc.Spy,
		)
		apiHandler.RegisterRoutes(router)
	})

	JustBeforeEach(func() {
		router.ServeHTTP(rr, req)
	})

	Describe("the GET /v3/apps/:guid endpoint", func() {
		BeforeEach(func() {
			appRepo.GetAppReturns(repositories.AppRecord{
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
			req, err = http.NewRequestWithContext(ctx, "GET", "/v3/apps/"+appGUID, nil)
			Expect(err).NotTo(HaveOccurred())
		})

		When("on the happy path", func() {
			It("returns status 200 OK", func() {
				Expect(rr.Code).To(Equal(http.StatusOK), "Matching HTTP response code:")
			})

			It("passes authInfo from context to GetApp", func() {
				Expect(appRepo.GetAppCallCount()).To(Equal(1))
				_, actualAuthInfo, _ := appRepo.GetAppArgsForCall(0)
				Expect(actualAuthInfo).To(Equal(authInfo))
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

		When("the app cannot be found or the client is not authorized to get it", func() {
			BeforeEach(func() {
				appRepo.GetAppReturns(repositories.AppRecord{}, repositories.PermissionDeniedOrNotFoundError{})
			})

			// TODO: should we return code 100004 instead?
			It("returns an error", func() {
				expectNotFoundError("App not found")
			})
		})

		When("there is some other error fetching the app", func() {
			BeforeEach(func() {
				appRepo.GetAppReturns(repositories.AppRecord{}, errors.New("unknown!"))
			})

			It("returns an error", func() {
				expectUnknownError()
			})
		})
	})

	Describe("the POST /v3/apps endpoint", func() {
		const (
			testAppName = "test-app"
		)

		queuePostRequest := func(requestBody string) {
			var err error
			req, err = http.NewRequestWithContext(ctx, "POST", "/v3/apps", strings.NewReader(requestBody))
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

			It("returns an error", func() {
				expectUnprocessableEntityError(`invalid request body: json: unknown field "description"`)
			})
		})

		When("the request body is invalid with invalid app name", func() {
			BeforeEach(func() {
				queuePostRequest(`{
					"name": 12345,
					"relationships": { "space": { "data": { "guid": "2f35885d-0c9d-4423-83ad-fd05066f8576" } } }
				}`)
			})

			It("returns an error", func() {
				expectUnprocessableEntityError("Name must be a string")
			})
		})

		When("the request body is invalid with invalid environment variable object", func() {
			BeforeEach(func() {
				queuePostRequest(`{
					"name": "my_app",
					"environment_variables": [],
					"relationships": { "space": { "data": { "guid": "2f35885d-0c9d-4423-83ad-fd05066f8576" } } }
				}`)
			})

			It("returns an error", func() {
				expectUnprocessableEntityError("Environment_variables must be a map[string]string")
			})
		})

		When("the request body is invalid with missing required name field", func() {
			BeforeEach(func() {
				queuePostRequest(`{
					"relationships": { "space": { "data": { "guid": "0c78dd5d-c723-4f2e-b168-df3c3e1d0806" } } }
				}`)
			})

			It("returns an error", func() {
				expectUnprocessableEntityError("Name is a required field")
			})
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
				appRepo.GetNamespaceReturns(
					repositories.SpaceRecord{},
					repositories.PermissionDeniedOrNotFoundError{Err: errors.New("not found")},
				)

				requestBody := initializeCreateAppRequestBody(testAppName, "no-such-guid", nil, nil, nil)
				queuePostRequest(requestBody)
			})

			It("returns an error", func() {
				expectUnprocessableEntityError("Invalid space. Ensure that the space exists and you have access to it.")
			})
		})

		When("the action errors due to validating webhook rejection", func() {
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

		When("the action errors due to a non webhook k8s error", func() {
			BeforeEach(func() {
				controllerError := new(k8serrors.StatusError)
				controllerError.ErrStatus.Reason = "different k8s api error"
				appRepo.CreateAppReturns(repositories.AppRecord{}, controllerError)

				requestBody := initializeCreateAppRequestBody(testAppName, spaceGUID, nil, nil, nil)
				queuePostRequest(requestBody)
			})

			It("returns an error", func() {
				expectUnknownError()
			})
		})

		When("the action errors due to an authorization error", func() {
			BeforeEach(func() {
				repoError := k8serrors.NewForbidden(schema.GroupResource{}, "forbidden", errors.New("foo"))
				appRepo.CreateAppReturns(repositories.AppRecord{}, repoError)

				requestBody := initializeCreateAppRequestBody(testAppName, spaceGUID, nil, nil, nil)
				queuePostRequest(requestBody)
			})

			It("returns an error", func() {
				expectUnauthorizedError()
			})
		})
	})

	Describe("the GET /v3/apps endpoint", func() {
		BeforeEach(func() {
			appRepo.ListAppsReturns([]repositories.AppRecord{
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
			req, err = http.NewRequestWithContext(ctx, "GET", "/v3/apps", nil)
			Expect(err).NotTo(HaveOccurred())
		})

		When("on the happy path", func() {
			When("Query Parameters are not provided", func() {
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
					"href": "%[1]s/v3/apps"
				  },
				  "last": {
					"href": "%[1]s/v3/apps"
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

			When("Query Parameters are provided", func() {
				BeforeEach(func() {
					var err error
					req, err = http.NewRequestWithContext(ctx, "GET", "/v3/apps?order_by=name", nil)
					Expect(err).NotTo(HaveOccurred())
				})

				It("returns status 200 OK", func() {
					Expect(rr.Code).Should(Equal(http.StatusOK), "Matching HTTP response code:")
				})
			})

			It("invokes the repository with the provided auth info", func() {
				Expect(appRepo.ListAppsCallCount()).To(Equal(1))
				_, actualAuthInfo, _ := appRepo.ListAppsArgsForCall(0)
				Expect(actualAuthInfo).To(Equal(authInfo))
			})

			When("filtering query params are provided", func() {
				BeforeEach(func() {
					var err error
					req, err = http.NewRequestWithContext(ctx, "GET", "/v3/apps?names=app1,app2&space_guids=space1,space2", nil)
					Expect(err).NotTo(HaveOccurred())
				})

				It("passes them to the repository", func() {
					Expect(appRepo.ListAppsCallCount()).To(Equal(1))
					_, _, message := appRepo.ListAppsArgsForCall(0)

					Expect(message.Names).To(ConsistOf("app1", "app2"))
					Expect(message.SpaceGuids).To(ConsistOf("space1", "space2"))
				})

				It("correctly sets query parameters in response pagination links", func() {
					Expect(rr.Body.String()).To(ContainSubstring("https://api.example.org/v3/apps?names=app1,app2&space_guids=space1,space2"))
				})
			})
		})

		When("no apps can be found", func() {
			BeforeEach(func() {
				appRepo.ListAppsReturns([]repositories.AppRecord{}, nil)
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
					"href": "%[1]s/v3/apps"
				  },
				  "last": {
					"href": "%[1]s/v3/apps"
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
				appRepo.ListAppsReturns([]repositories.AppRecord{}, errors.New("unknown!"))
			})

			It("returns an error", func() {
				expectUnknownError()
			})
		})

		When("invalid query parameters are provided", func() {
			BeforeEach(func() {
				var err error
				req, err = http.NewRequestWithContext(ctx, "GET", "/v3/apps?foo=bar", nil)
				Expect(err).NotTo(HaveOccurred())
			})

			It("returns an Unknown key error", func() {
				expectUnknownKeyError("The query parameter is invalid: Valid parameters are: 'names, guids, space_guids, order_by'")
			})
		})

		When("no auth info is present in the context", func() {
			BeforeEach(func() {
				var err error
				req, err = http.NewRequest("GET", "/v3/apps", nil)
				Expect(err).NotTo(HaveOccurred())
			})

			It("returns an Unknown error", func() {
				expectUnknownError()
			})
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

			appRepo.GetAppReturns(app, nil)
			dropletRepo.GetDropletReturns(droplet, nil)
			appRepo.SetCurrentDropletReturns(repositories.CurrentDropletRecord{
				AppGUID:     appGUID,
				DropletGUID: dropletGUID,
			}, nil)

			var err error
			req, err = http.NewRequestWithContext(ctx, "PATCH", "/v3/apps/"+appGUID+"/relationships/current_droplet", strings.NewReader(`
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
				Expect(appRepo.GetAppCallCount()).To(Equal(1))
				_, _, actualAppGUID := appRepo.GetAppArgsForCall(0)
				Expect(actualAppGUID).To(Equal(appGUID))
			})

			It("fetches the right Droplet", func() {
				Expect(dropletRepo.GetDropletCallCount()).To(Equal(1))
				_, _, actualDropletGUID := dropletRepo.GetDropletArgsForCall(0)
				Expect(actualDropletGUID).To(Equal(dropletGUID))
			})
		})

		When("the user client factory returns not auth'd", func() {
			BeforeEach(func() {
				appRepo.SetCurrentDropletReturns(repositories.CurrentDropletRecord{}, fmt.Errorf("foo: %w", authorization.NotAuthenticatedError{}))
			})

			It("returns a not authenticated error", func() {
				expectNotAuthenticatedError()
			})
		})

		When("the user client factory returns invalid auth", func() {
			BeforeEach(func() {
				appRepo.SetCurrentDropletReturns(repositories.CurrentDropletRecord{}, fmt.Errorf("foo: %w", authorization.InvalidAuthError{}))
			})

			It("returns a not authenticated error", func() {
				expectInvalidAuthError()
			})
		})

		When("set droplet is forbidden", func() {
			BeforeEach(func() {
				appRepo.SetCurrentDropletReturns(repositories.CurrentDropletRecord{}, repositories.NewForbiddenError(nil))
			})

			It("returns a not authenticated error", func() {
				expectUnauthorizedError()
			})
		})

		When("the App doesn't exist", func() {
			BeforeEach(func() {
				appRepo.GetAppReturns(repositories.AppRecord{}, repositories.PermissionDeniedOrNotFoundError{})
			})

			It("returns an error", func() {
				expectNotFoundError("App not found")
			})
			itDoesntSetTheCurrentDroplet()
		})

		When("the Droplet doesn't exist", func() {
			BeforeEach(func() {
				dropletRepo.GetDropletReturns(repositories.DropletRecord{}, repositories.NotFoundError{})
			})

			It("returns an error", func() {
				expectUnprocessableEntityError("Unable to assign current droplet. Ensure the droplet exists and belongs to this app.")
			})
			itDoesntSetTheCurrentDroplet()
		})

		When("the Droplet isn't accessible to the user", func() {
			BeforeEach(func() {
				dropletRepo.GetDropletReturns(repositories.DropletRecord{}, repositories.ForbiddenError{})
			})

			It("returns an error", func() {
				expectUnprocessableEntityError("Unable to assign current droplet. Ensure the droplet exists and belongs to this app.")
			})
			itDoesntSetTheCurrentDroplet()
		})

		When("the Droplet belongs to a different App", func() {
			BeforeEach(func() {
				droplet.AppGUID = "a-different-app-guid"
				dropletRepo.GetDropletReturns(repositories.DropletRecord{
					GUID:    dropletGUID,
					AppGUID: "a-different-app-guid",
				}, nil)
			})

			It("returns an error", func() {
				expectUnprocessableEntityError("Unable to assign current droplet. Ensure the droplet exists and belongs to this app.")
			})
			itDoesntSetTheCurrentDroplet()
		})

		When("the guid is missing", func() {
			BeforeEach(func() {
				var err error
				req, err = http.NewRequestWithContext(ctx, "PATCH", "/v3/apps/"+appGUID+"/relationships/current_droplet", strings.NewReader(`
					{ "data": {  } }
                `))
				Expect(err).NotTo(HaveOccurred())
			})

			It("returns an error", func() {
				expectUnprocessableEntityError("GUID is a required field")
			})
		})

		When("fetching the App errors", func() {
			BeforeEach(func() {
				appRepo.GetAppReturns(repositories.AppRecord{}, errors.New("boom"))
			})

			It("returns an error", func() {
				expectUnknownError()
			})
			itDoesntSetTheCurrentDroplet()
		})

		When("fetching the Droplet errors", func() {
			BeforeEach(func() {
				dropletRepo.GetDropletReturns(repositories.DropletRecord{}, errors.New("boom"))
			})

			It("returns an error", func() {
				expectUnknownError()
			})
			itDoesntSetTheCurrentDroplet()
		})

		When("setting the current droplet errors", func() {
			BeforeEach(func() {
				appRepo.SetCurrentDropletReturns(repositories.CurrentDropletRecord{}, errors.New("boom"))
			})

			It("returns an error", func() {
				expectUnknownError()
			})
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
			appRepo.GetAppReturns(fetchAppRecord, nil)
			setAppDesiredStateRecord := fetchAppRecord
			setAppDesiredStateRecord.State = "STARTED"
			appRepo.SetAppDesiredStateReturns(setAppDesiredStateRecord, nil)

			var err error
			req, err = http.NewRequestWithContext(ctx, "POST", "/v3/apps/"+appGUID+"/actions/start", nil)
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
				appRepo.GetAppReturns(repositories.AppRecord{}, repositories.NotFoundError{})
			})

			// TODO: should we return code 100004 instead?
			It("returns an error", func() {
				expectNotFoundError("App not found")
			})
		})

		When("there is some other error fetching the app", func() {
			BeforeEach(func() {
				appRepo.GetAppReturns(repositories.AppRecord{}, errors.New("unknown!"))
			})

			It("returns an error", func() {
				expectUnknownError()
			})
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
				appRepo.GetAppReturns(fetchAppRecord, nil)
			})

			It("returns an error", func() {
				expectUnprocessableEntityError(`Assign a droplet before starting this app.`)
			})
		})

		When("there is some other error updating app desiredState", func() {
			BeforeEach(func() {
				appRepo.SetAppDesiredStateReturns(repositories.AppRecord{}, errors.New("unknown!"))
			})

			It("returns an error", func() {
				expectUnknownError()
			})
		})
	})

	Describe("the POST /v3/apps/:guid/actions/stop endpoint", func() {
		BeforeEach(func() {
			fetchAppRecord := repositories.AppRecord{
				Name:        appName,
				GUID:        appGUID,
				SpaceGUID:   spaceGUID,
				DropletGUID: "some-droplet-guid",
				State:       "STARTED",
				Lifecycle: repositories.Lifecycle{
					Type: "buildpack",
					Data: repositories.LifecycleData{
						Buildpacks: []string{},
						Stack:      "",
					},
				},
			}
			appRepo.GetAppReturns(fetchAppRecord, nil)
			setAppDesiredStateRecord := fetchAppRecord
			setAppDesiredStateRecord.State = "STOPPED"
			appRepo.SetAppDesiredStateReturns(setAppDesiredStateRecord, nil)

			var err error
			req, err = http.NewRequestWithContext(ctx, "POST", "/v3/apps/"+appGUID+"/actions/stop", nil)
			Expect(err).NotTo(HaveOccurred())
		})

		When("when the app is STARTED", func() {
			It("returns status 200 OK", func() {
				Expect(rr.Code).To(Equal(http.StatusOK), "Matching HTTP response code:")
			})

			It("returns the App in the response with a state of STOPPED", func() {
				contentTypeHeader := rr.Header().Get("Content-Type")
				Expect(contentTypeHeader).To(Equal(jsonHeader), "Matching Content-Type header:")

				Expect(rr.Body.String()).To(MatchJSON(fmt.Sprintf(`{
					"guid": "%[2]s",
					"created_at": "",
					"updated_at": "",
					"name": "%[4]s",
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
				}`, defaultServerURL, appGUID, spaceGUID, appName)), "Response body matches response:")
			})
		})

		When("when the app is STOPPED", func() {
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
				appRepo.GetAppReturns(fetchAppRecord, nil)
				setAppDesiredStateRecord := fetchAppRecord
				setAppDesiredStateRecord.State = "STOPPED"
				appRepo.SetAppDesiredStateReturns(setAppDesiredStateRecord, nil)

				var err error
				req, err = http.NewRequestWithContext(ctx, "POST", "/v3/apps/"+appGUID+"/actions/stop", nil)
				Expect(err).NotTo(HaveOccurred())
			})

			It("returns status 200 OK", func() {
				Expect(rr.Code).To(Equal(http.StatusOK), "Matching HTTP response code:")
			})

			It("returns the App in the response with a state of STOPPED", func() {
				contentTypeHeader := rr.Header().Get("Content-Type")
				Expect(contentTypeHeader).To(Equal(jsonHeader), "Matching Content-Type header:")

				Expect(rr.Body.String()).To(MatchJSON(fmt.Sprintf(`{
					"guid": "%[2]s",
					"created_at": "",
					"updated_at": "",
					"name": "%[4]s",
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
				}`, defaultServerURL, appGUID, spaceGUID, appName)), "Response body matches response:")
			})
		})

		When("the app cannot be found", func() {
			BeforeEach(func() {
				appRepo.GetAppReturns(repositories.AppRecord{}, repositories.NotFoundError{})
			})

			It("returns an error", func() {
				expectNotFoundError("App not found")
			})
		})

		When("there is some other error fetching the app", func() {
			BeforeEach(func() {
				appRepo.GetAppReturns(repositories.AppRecord{}, errors.New("unknown!"))
			})

			It("returns an error", func() {
				expectUnknownError()
			})
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
				appRepo.GetAppReturns(fetchAppRecord, nil)
			})

			It("returns status 200 OK", func() {
				Expect(rr.Code).To(Equal(http.StatusOK), "Matching HTTP response code:")
			})

			It("returns the App in the response with a state of STOPPED", func() {
				contentTypeHeader := rr.Header().Get("Content-Type")
				Expect(contentTypeHeader).To(Equal(jsonHeader), "Matching Content-Type header:")

				Expect(rr.Body.String()).To(MatchJSON(fmt.Sprintf(`{
					"guid": "%[2]s",
					"created_at": "",
					"updated_at": "",
					"name": "%[4]s",
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
				}`, defaultServerURL, appGUID, spaceGUID, appName)), "Response body matches response:")
			})
		})

		When("there is some other error updating app desiredState", func() {
			BeforeEach(func() {
				appRepo.SetAppDesiredStateReturns(repositories.AppRecord{}, errors.New("unknown!"))
			})

			It("returns an error", func() {
				expectUnknownError()
			})
		})
	})

	Describe("the GET /v3/apps/:guid/processes endpoint", func() {
		var (
			process1Record *repositories.ProcessRecord
			process2Record *repositories.ProcessRecord
		)
		BeforeEach(func() {
			processRecord := repositories.ProcessRecord{
				GUID:             "process-1-guid",
				SpaceGUID:        spaceGUID,
				AppGUID:          appGUID,
				Type:             "web",
				Command:          "rackup",
				DesiredInstances: 5,
				MemoryMB:         256,
				DiskQuotaMB:      1024,
				Ports:            []int32{8080},
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
			processRecord2.DesiredInstances = 1
			processRecord2.HealthCheck.Type = "process"

			process1Record = &processRecord
			process2Record = &processRecord2
			processRepo.ListProcessesReturns([]repositories.ProcessRecord{
				processRecord,
				processRecord2,
			}, nil)

			var err error
			req, err = http.NewRequestWithContext(ctx, "GET", "/v3/apps/"+appGUID+"/processes", nil)
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
							"href": "%[1]s/v3/apps/%[2]s/processes"
						  },
						  "last": {
							"href": "%[1]s/v3/apps/%[2]s/processes"
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
					processRepo.ListProcessesReturns([]repositories.ProcessRecord{}, nil)
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
							"href": "%[1]s/v3/apps/%[2]s/processes"
						  },
						  "last": {
							"href": "%[1]s/v3/apps/%[2]s/processes"
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
			When("the app cannot be found or user is not authorized to see it", func() {
				BeforeEach(func() {
					appRepo.GetAppReturns(repositories.AppRecord{}, repositories.PermissionDeniedOrNotFoundError{})
				})

				It("returns an error", func() {
					expectNotFoundError("App not found")
				})
			})

			When("there is some other error fetching the app", func() {
				BeforeEach(func() {
					appRepo.GetAppReturns(repositories.AppRecord{}, errors.New("unknown!"))
				})

				It("returns an error", func() {
					expectUnknownError()
				})
			})
			When("there is some error fetching the app's processes", func() {
				BeforeEach(func() {
					processRepo.ListProcessesReturns([]repositories.ProcessRecord{}, errors.New("unknown!"))
				})

				It("returns an error", func() {
					expectUnknownError()
				})
			})
		})
	})

	Describe("the POST /v3/apps/:guid/process/:processType/actions/scale endpoint", func() {
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
			req, err = http.NewRequestWithContext(ctx, "POST", "/v3/apps/"+appGUID+"/processes/"+processType+"/actions/scale", strings.NewReader(requestBody))
			Expect(err).NotTo(HaveOccurred())
		}

		BeforeEach(func() {
			scaleAppProcessFunc.Returns(repositories.ProcessRecord{
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
					Expect(scaleAppProcessFunc.CallCount()).To(Equal(1), "did not call scaleProcess just once")
					_, _, _, _, invokedProcessScale := scaleAppProcessFunc.ArgsForCall(0)
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
					scaleAppProcessFunc.Returns(repositories.ProcessRecord{}, repositories.NotFoundError{ResourceType: "Process"})
				})

				It("returns an error", func() {
					expectNotFoundError("Process not found")
				})
			})

			When("the app doesn't exist", func() {
				BeforeEach(func() {
					scaleAppProcessFunc.Returns(repositories.ProcessRecord{}, repositories.NotFoundError{ResourceType: "App"})
				})

				It("returns an error", func() {
					expectNotFoundError("App not found")
				})
			})

			When("there is some other error fetching the process", func() {
				BeforeEach(func() {
					scaleAppProcessFunc.Returns(repositories.ProcessRecord{}, errors.New("unknown!"))
				})

				It("returns an error", func() {
					expectUnknownError()
				})
			})
		})

		When("the validating scale parameters", func() {
			DescribeTable("returns validation",
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

	Describe("the GET /v3/apps/:guid/routes endpoint", func() {
		const (
			testDomainGUID = "test-domain-guid"
			testRouteGUID  = "test-route-guid"
			testRouteHost  = "test-route-host"
			testSpaceGUID  = "test-space-guid"
		)

		var (
			route1Record *repositories.RouteRecord

			domainRecord *repositories.DomainRecord
		)

		BeforeEach(func() {
			appRepo.GetAppReturns(repositories.AppRecord{GUID: appGUID, SpaceGUID: testSpaceGUID}, nil)

			routeRecord := repositories.RouteRecord{
				GUID:      testRouteGUID,
				SpaceGUID: testSpaceGUID,
				Domain: repositories.DomainRecord{
					GUID: testDomainGUID,
				},

				Host:         testRouteHost,
				Path:         "/some_path",
				Protocol:     "http",
				Destinations: nil,
				Labels:       nil,
				Annotations:  nil,
				CreatedAt:    "2019-05-10T17:17:48Z",
				UpdatedAt:    "2019-05-10T17:17:48Z",
			}

			route1Record = &routeRecord
			routeRepo.ListRoutesForAppReturns([]repositories.RouteRecord{
				routeRecord,
			}, nil)

			domainRecord = &repositories.DomainRecord{
				GUID: testDomainGUID,
				Name: "example.org",
			}
			domainRepo.GetDomainReturns(*domainRecord, nil)

			var err error
			req, err = http.NewRequestWithContext(ctx, "GET", "/v3/apps/"+appGUID+"/routes", nil)
			Expect(err).NotTo(HaveOccurred())
		})

		When("on the happy path and", func() {
			It("sends authInfo from the context to the repo methods", func() {
				Expect(appRepo.GetAppCallCount()).To(Equal(1))
				_, actualAuthInfo, _ := appRepo.GetAppArgsForCall(0)
				Expect(actualAuthInfo).To(Equal(authInfo))

				Expect(routeRepo.ListRoutesForAppCallCount()).To(Equal(1))
				_, actualAuthInfo, _, _ = routeRepo.ListRoutesForAppArgsForCall(0)
				Expect(actualAuthInfo).To(Equal(authInfo))

				Expect(domainRepo.GetDomainCallCount()).To(Equal(1))
				_, actualAuthInfo, _ = domainRepo.GetDomainArgsForCall(0)
				Expect(actualAuthInfo).To(Equal(authInfo))
			})

			When("the App has associated routes", func() {
				It("returns status 200 OK", func() {
					Expect(rr.Code).To(Equal(http.StatusOK), "Matching HTTP response code:")
				})

				It("returns Content-Type as JSON in header", func() {
					contentTypeHeader := rr.Header().Get("Content-Type")
					Expect(contentTypeHeader).To(Equal(jsonHeader), "Matching Content-Type header:")
				})

				It("returns the Pagination Data and App Resources in the response", func() {
					Expect(rr.Body.String()).To(MatchJSON(fmt.Sprintf(`{
						"pagination": {
							"total_results": 1,
							"total_pages": 1,
							"first": {
								"href": "%[1]s/v3/apps/%[2]s/routes"
							},
							"last": {
								"href": "%[1]s/v3/apps/%[2]s/routes"
							},
							"next": null,
							"previous": null
						},
						"resources": [
							{
								"guid": "%[3]s",
								"port": null,
								"path": "%[4]s",
								"protocol": "%[5]s",
								"host": "%[6]s",
								"url": "%[6]s.%[7]s%[4]s",
								"created_at": "%[8]s",
								"updated_at": "%[9]s",
								"destinations": [],
								"relationships": {
									"space": {
										"data": {
											"guid": "%[10]s"
										}
									},
									"domain": {
										"data": {
											"guid": "%[11]s"
										}
									}
								},
								"metadata": {
									"labels": {},
									"annotations": {}
								},
								"links": {
									"self":{
										"href": "%[1]s/v3/routes/%[3]s"
									},
									"space":{
										"href": "%[1]s/v3/spaces/%[10]s"
									},
									"domain":{
										"href": "%[1]s/v3/domains/%[11]s"
									},
									"destinations":{
										"href": "%[1]s/v3/routes/%[3]s/destinations"
									}
								}
							}
						]
					}`, defaultServerURL, appGUID, route1Record.GUID, route1Record.Path, route1Record.Protocol, route1Record.Host, domainRecord.Name, route1Record.CreatedAt, route1Record.UpdatedAt, route1Record.SpaceGUID, domainRecord.GUID)), "Response body matches response:")
				})
			})

			When("The App does not have associated routes", func() {
				BeforeEach(func() {
					routeRepo.ListRoutesForAppReturns([]repositories.RouteRecord{}, nil)
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
							"href": "%[1]s/v3/apps/%[2]s/routes"
						  },
						  "last": {
							"href": "%[1]s/v3/apps/%[2]s/routes"
						  },
						  "next": null,
						  "previous": null
						},
						"resources": []
					}`, defaultServerURL, appGUID)), "Response body matches response:")
				})
			})
		})

		When("on the sad path and", func() {
			When("the app cannot be found or no permission", func() {
				BeforeEach(func() {
					appRepo.GetAppReturns(repositories.AppRecord{}, repositories.PermissionDeniedOrNotFoundError{})
				})

				It("returns an error", func() {
					expectNotFoundError("App not found")
				})
			})

			When("there is some other error fetching the app", func() {
				BeforeEach(func() {
					appRepo.GetAppReturns(repositories.AppRecord{}, errors.New("unknown!"))
				})

				It("returns an error", func() {
					expectUnknownError()
				})
			})

			When("there is some error fetching the app's routes", func() {
				BeforeEach(func() {
					routeRepo.ListRoutesForAppReturns([]repositories.RouteRecord{}, errors.New("unknown!"))
				})

				It("returns an error", func() {
					expectUnknownError()
				})
			})
		})
	})

	Describe("the GET /v3/apps/:guid/droplets/current", func() {
		const (
			dropletGUID = "test-droplet-guid"
			packageGUID = "test-package-guid"
		)

		var (
			app       repositories.AppRecord
			droplet   repositories.DropletRecord
			timestamp string
		)

		BeforeEach(func() {
			app = repositories.AppRecord{GUID: appGUID, SpaceGUID: spaceGUID, DropletGUID: dropletGUID}
			timestamp = time.Unix(1631892190, 0).String()
			droplet = repositories.DropletRecord{
				GUID:      dropletGUID,
				State:     "STAGED",
				CreatedAt: timestamp,
				UpdatedAt: timestamp,
				Lifecycle: repositories.Lifecycle{
					Type: "buildpack",
					Data: repositories.LifecycleData{
						Buildpacks: []string{},
						Stack:      "",
					},
				},
				Stack: "cflinuxfs3",
				ProcessTypes: map[string]string{
					"rake": "bundle exec rake",
					"web":  "bundle exec rackup config.ru -p $PORT",
				},
				AppGUID:     appGUID,
				PackageGUID: packageGUID,
			}

			appRepo.GetAppReturns(app, nil)
			dropletRepo.GetDropletReturns(droplet, nil)

			var err error
			req, err = http.NewRequestWithContext(ctx, "GET", "/v3/apps/"+appGUID+"/droplets/current", nil)
			Expect(err).NotTo(HaveOccurred())
		})

		When("on the happy path", func() {
			It("responds with a 200 code", func() {
				Expect(rr.Code).To(Equal(200))
			})

			It("sends the authInfo from the context to the repo methods", func() {
				Expect(appRepo.GetAppCallCount()).To(Equal(1))
				_, actualAuthInfo, _ := appRepo.GetAppArgsForCall(0)
				Expect(actualAuthInfo).To(Equal(authInfo))

				Expect(dropletRepo.GetDropletCallCount()).To(Equal(1))
				_, actualAuthInfo, _ = dropletRepo.GetDropletArgsForCall(0)
				Expect(actualAuthInfo).To(Equal(authInfo))
			})

			It("responds with the current droplet encoded as JSON", func() {
				contentTypeHeader := rr.Header().Get("Content-Type")
				Expect(contentTypeHeader).To(Equal(jsonHeader), "Matching Content-Type header:")

				Expect(rr.Body.String()).To(MatchJSON(`{
					  "guid": "`+dropletGUID+`",
					  "state": "STAGED",
					  "error": null,
					  "lifecycle": {
						"type": "buildpack",
						"data": {
							"buildpacks": [],
							"stack": ""
						}
					},
					  "execution_metadata": "",
					  "process_types": {
						"rake": "bundle exec rake",
						"web": "bundle exec rackup config.ru -p $PORT"
					  },
					  "checksum": null,
					  "buildpacks": [],
					  "stack": "cflinuxfs3",
					  "image": null,
					  "created_at": "`+timestamp+`",
					  "updated_at": "`+timestamp+`",
					  "relationships": {
						"app": {
						  "data": {
							"guid": "`+appGUID+`"
						  }
						}
					  },
					  "links": {
						"self": {
						  "href": "`+defaultServerURI("/v3/droplets/", dropletGUID)+`"
						},
						"package": {
						  "href": "`+defaultServerURI("/v3/packages/", packageGUID)+`"
						},
						"app": {
						  "href": "`+defaultServerURI("/v3/apps/", appGUID)+`"
						},
						"assign_current_droplet": {
						  "href": "`+defaultServerURI("/v3/apps/", appGUID, "/relationships/current_droplet")+`",
						  "method": "PATCH"
						  },
						"download": null
					  },
					  "metadata": {
						"labels": {},
						"annotations": {}
					  }
					}`), "Response body matches response:")
			})

			It("fetches the correct App", func() {
				Expect(appRepo.GetAppCallCount()).To(Equal(1))
				_, _, actualAppGUID := appRepo.GetAppArgsForCall(0)
				Expect(actualAppGUID).To(Equal(appGUID))
			})

			It("fetches the correct Droplet", func() {
				Expect(dropletRepo.GetDropletCallCount()).To(Equal(1))
				_, _, actualDropletGUID := dropletRepo.GetDropletArgsForCall(0)
				Expect(actualDropletGUID).To(Equal(dropletGUID))
			})
		})

		When("the App doesn't exist", func() {
			BeforeEach(func() {
				appRepo.GetAppReturns(repositories.AppRecord{}, repositories.PermissionDeniedOrNotFoundError{})
			})

			It("returns an error", func() {
				expectNotFoundError("App not found")
			})
		})

		When("the App doesn't have a current droplet assigned", func() {
			BeforeEach(func() {
				appRepo.GetAppReturns(repositories.AppRecord{GUID: appGUID, SpaceGUID: spaceGUID, DropletGUID: ""}, nil)
			})

			It("returns an error", func() {
				expectNotFoundError("Droplet not found")
			})
		})

		When("the Droplet doesn't exist", func() {
			BeforeEach(func() {
				dropletRepo.GetDropletReturns(repositories.DropletRecord{}, repositories.NotFoundError{})
			})

			It("returns an error", func() {
				expectNotFoundError("Droplet not found")
			})
		})

		When("the user cannot access the droplet", func() {
			BeforeEach(func() {
				dropletRepo.GetDropletReturns(repositories.DropletRecord{}, repositories.NewForbiddenError(nil))
			})

			It("returns an error", func() {
				expectNotFoundError("Droplet not found")
			})
		})

		When("fetching the App errors", func() {
			BeforeEach(func() {
				appRepo.GetAppReturns(repositories.AppRecord{}, errors.New("boom"))
			})

			It("returns an error", func() {
				expectUnknownError()
			})
		})

		When("fetching the Droplet errors", func() {
			BeforeEach(func() {
				dropletRepo.GetDropletReturns(repositories.DropletRecord{}, errors.New("boom"))
			})

			It("returns an error", func() {
				expectUnknownError()
			})
		})
	})

	Describe("the GET /v3/apps/:guid/actions/restart endpoint", func() {
		var fetchAppRecord repositories.AppRecord
		BeforeEach(func() {
			fetchAppRecord = repositories.AppRecord{
				Name:        appName,
				GUID:        appGUID,
				SpaceGUID:   spaceGUID,
				DropletGUID: "some-droplet-guid",
				State:       "STARTED",
				Lifecycle: repositories.Lifecycle{
					Type: "buildpack",
					Data: repositories.LifecycleData{
						Buildpacks: []string{},
						Stack:      "",
					},
				},
			}
			appRepo.GetAppReturns(fetchAppRecord, nil)
			setAppDesiredStateRecord := fetchAppRecord
			setAppDesiredStateRecord.State = "STARTED"
			appRepo.SetAppDesiredStateReturns(setAppDesiredStateRecord, nil)

			var err error
			req, err = http.NewRequestWithContext(ctx, "POST", "/v3/apps/"+appGUID+"/actions/restart", nil)
			Expect(err).NotTo(HaveOccurred())
		})

		It("sends the authInfo from the context to the repo methods", func() {
			Expect(appRepo.GetAppCallCount()).To(Equal(1))
			_, actualAuthInfo, _ := appRepo.GetAppArgsForCall(0)
			Expect(actualAuthInfo).To(Equal(authInfo))

			Expect(appRepo.SetAppDesiredStateCallCount()).To(Equal(2))
			_, actualAuthInfo, _ = appRepo.SetAppDesiredStateArgsForCall(0)
			Expect(actualAuthInfo).To(Equal(authInfo))
			_, actualAuthInfo, _ = appRepo.SetAppDesiredStateArgsForCall(1)
			Expect(actualAuthInfo).To(Equal(authInfo))
		})

		When("no permissions to get the app or the app cannot be found", func() {
			BeforeEach(func() {
				appRepo.GetAppReturns(repositories.AppRecord{}, repositories.PermissionDeniedOrNotFoundError{})
			})

			It("returns an error", func() {
				expectNotFoundError("App not found")
			})
		})

		When("there is some other error fetching the app", func() {
			BeforeEach(func() {
				appRepo.GetAppReturns(repositories.AppRecord{}, errors.New("unknown!"))
			})

			It("returns an error", func() {
				expectUnknownError()
			})
		})

		When("the app has no droplet", func() {
			BeforeEach(func() {
				noDropletAppRecord := repositories.AppRecord{
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
				appRepo.GetAppReturns(noDropletAppRecord, nil)
			})

			It("returns an error", func() {
				expectUnprocessableEntityError(`Assign a droplet before starting this app.`)
			})
		})

		When("the app is in STARTED state", func() {
			It("responds with a 200 code", func() {
				Expect(rr.Code).To(Equal(200))
			})

			It("calls setDesiredState to STOP and START the app", func() {
				Expect(appRepo.SetAppDesiredStateCallCount()).To(Equal(2))

				_, _, appDesiredStateMessage := appRepo.SetAppDesiredStateArgsForCall(0)
				Expect(appDesiredStateMessage.DesiredState).To(Equal("STOPPED"))

				_, _, appDesiredStateMessage = appRepo.SetAppDesiredStateArgsForCall(1)
				Expect(appDesiredStateMessage.DesiredState).To(Equal("STARTED"))
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

			When("no permissions to stop the app", func() {
				BeforeEach(func() {
					appRepo.SetAppDesiredStateReturnsOnCall(0, repositories.AppRecord{}, repositories.PermissionDeniedOrNotFoundError{})
				})

				It("returns a forbidden error", func() {
					expectUnauthorizedError()
				})
			})

			When("no permissions to start the app", func() {
				BeforeEach(func() {
					appRepo.SetAppDesiredStateReturnsOnCall(1, repositories.AppRecord{}, repositories.PermissionDeniedOrNotFoundError{})
				})

				It("returns a forbidden error", func() {
					expectUnauthorizedError()
				})
			})
		})

		When("the app is in STOPPED state", func() {
			BeforeEach(func() {
				fetchAppRecord.State = "STOPPED"
				appRepo.GetAppReturns(fetchAppRecord, nil)
			})
			It("responds with a 200 code", func() {
				Expect(rr.Code).To(Equal(200))
			})

			It("only calls setDesiredState to START the app", func() {
				Expect(appRepo.SetAppDesiredStateCallCount()).To(Equal(1))
				_, _, appDesiredStateMessage := appRepo.SetAppDesiredStateArgsForCall(0)
				Expect(appDesiredStateMessage.DesiredState).To(Equal("STARTED"))
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

		When("no permissions to start the app", func() {
			BeforeEach(func() {
				appRepo.SetAppDesiredStateReturns(repositories.AppRecord{}, repositories.PermissionDeniedOrNotFoundError{})
			})

			It("returns a forbidden error", func() {
				expectUnauthorizedError()
			})
		})

		When("setDesiredAppState to STOPPED returns an error", func() {
			BeforeEach(func() {
				appRepo.SetAppDesiredStateReturnsOnCall(0, repositories.AppRecord{}, errors.New("some-error"))
			})

			It("returns an error", func() {
				expectUnknownError()
			})
		})

		When("setDesiredAppState to STARTED returns an error", func() {
			BeforeEach(func() {
				appRepo.SetAppDesiredStateReturnsOnCall(1, repositories.AppRecord{}, errors.New("some-error"))
			})

			It("returns an error", func() {
				expectUnknownError()
			})
		})
	})

	Describe("the DELETE /v3/apps/:guid endpoint", func() {
		var app repositories.AppRecord

		BeforeEach(func() {
			app = repositories.AppRecord{GUID: appGUID, SpaceGUID: spaceGUID}

			appRepo.GetAppReturns(app, nil)

			var err error
			req, err = http.NewRequestWithContext(ctx, "DELETE", "/v3/apps/"+appGUID, nil)
			Expect(err).NotTo(HaveOccurred())
		})

		When("on the happy path", func() {
			It("responds with a 202 accepted response", func() {
				Expect(rr.Code).To(Equal(http.StatusAccepted))
			})

			It("responds with a job URL in a location header", func() {
				locationHeader := rr.Header().Get("Location")
				Expect(locationHeader).To(Equal("https://api.example.org/v3/jobs/app.delete-"+appGUID), "Matching Location header")
			})

			It("fetches the right App", func() {
				Expect(appRepo.GetAppCallCount()).To(Equal(1))
				_, _, actualAppGUID := appRepo.GetAppArgsForCall(0)
				Expect(actualAppGUID).To(Equal(appGUID))
			})

			It("deletes the K8s record via the repository", func() {
				Expect(appRepo.DeleteAppCallCount()).To(Equal(1))
				_, _, message := appRepo.DeleteAppArgsForCall(0)
				Expect(message.AppGUID).To(Equal(appGUID))
				Expect(message.SpaceGUID).To(Equal(spaceGUID))
			})
		})

		When("the App doesn't exist", func() {
			BeforeEach(func() {
				appRepo.GetAppReturns(repositories.AppRecord{}, repositories.NotFoundError{})
			})

			It("returns an error", func() {
				expectNotFoundError("App not found")
			})
		})

		When("fetching the App errors", func() {
			BeforeEach(func() {
				appRepo.GetAppReturns(repositories.AppRecord{}, errors.New("boom"))
			})

			It("returns an error", func() {
				expectUnknownError()
			})
		})

		When("deleting the App errors", func() {
			BeforeEach(func() {
				appRepo.DeleteAppReturns(errors.New("boom"))
			})

			It("returns an error", func() {
				expectUnknownError()
			})
		})
	})

	Describe("the GET /v3/apps/:guid/env endpoint", func() {
		BeforeEach(func() {
			appRepo.GetAppEnvReturns(map[string]string{"VAR": "VAL"}, nil)

			var err error
			req, err = http.NewRequestWithContext(ctx, "GET", "/v3/apps/"+appGUID+"/env", nil)
			Expect(err).NotTo(HaveOccurred())
		})

		When("on the happy path", func() {
			It("returns status 200 OK", func() {
				Expect(rr.Code).To(Equal(http.StatusOK), "Matching HTTP response code:")
			})

			It("passes authInfo from context to GetAppEnv", func() {
				Expect(appRepo.GetAppEnvCallCount()).To(Equal(1))
				_, actualAuthInfo, _ := appRepo.GetAppEnvArgsForCall(0)
				Expect(actualAuthInfo).To(Equal(authInfo))
			})

			It("returns the env vars in the response", func() {
				contentTypeHeader := rr.Header().Get("Content-Type")
				Expect(contentTypeHeader).To(Equal(jsonHeader), "Matching Content-Type header:")

				Expect(rr.Body.String()).To(MatchJSON(`{
                  "staging_env_json": {},
                  "running_env_json": {},
                  "environment_variables": { "VAR": "VAL" },
                  "system_env_json": {},
                  "application_env_json": {}
                }`))
			})
		})

		When("the app cannot be found", func() {
			BeforeEach(func() {
				appRepo.GetAppEnvReturns(nil, repositories.NotFoundError{ResourceType: "App"})
			})

			It("returns an error", func() {
				expectNotFoundError("App not found")
			})
		})

		When("there is a Forbidden error fetching the app env", func() {
			BeforeEach(func() {
				appRepo.GetAppEnvReturns(nil, repositories.ForbiddenError{})
			})

			It("returns an error", func() {
				expectUnauthorizedError()
			})
		})

		When("there is some other error fetching the app env", func() {
			BeforeEach(func() {
				appRepo.GetAppEnvReturns(nil, errors.New("unknown!"))
			})

			It("returns an error", func() {
				expectUnknownError()
			})
		})
	})

	Describe("the PATCH /v3/apps/:guid/environment_variables", func() {
		const (
			key0 = "KEY0"
			key1 = "KEY1"
			key2 = "KEY2"
		)

		queuePatchRequest := func(appGUID, requestBody string) {
			var err error
			req, err = http.NewRequestWithContext(ctx, "PATCH", "/v3/apps/"+appGUID+"/environment_variables", strings.NewReader(requestBody))
			Expect(err).NotTo(HaveOccurred())
		}

		var (
			appReturns       repositories.AppRecord
			appSecretReturns repositories.AppEnvVarsRecord
			appEnvVars       map[string]string
		)

		BeforeEach(func() {
			appReturns = repositories.AppRecord{GUID: appGUID, SpaceGUID: spaceGUID}
			appRepo.GetAppReturns(appReturns, nil)

			appEnvVars = map[string]string{
				key0: "VAL0",
				key2: "VAL2",
			}
			appSecretReturns = repositories.AppEnvVarsRecord{
				Name:                 appGUID + "-env",
				AppGUID:              appGUID,
				SpaceGUID:            spaceGUID,
				EnvironmentVariables: appEnvVars,
			}
			appRepo.PatchAppEnvVarsReturns(appSecretReturns, nil)
			queuePatchRequest(appGUID, `{ "var": { "`+key1+`": null, "`+key2+`": "VAL2" } }`)
		})

		When("on the happy path", func() {
			It("responds with a 200 code", func() {
				Expect(rr.Code).To(Equal(200))
			})

			It("responds with JSON", func() {
				contentTypeHeader := rr.Header().Get("Content-Type")
				Expect(contentTypeHeader).To(Equal(jsonHeader), "Matching Content-Type header:")

				Expect(rr.Body.String()).To(MatchJSON(fmt.Sprintf(`{
					"var": {
						"KEY0": "VAL0",
						"KEY2": "VAL2"
					},
					"links": {
						"self": {
							"href": "%[1]s/v3/apps/%[2]s/environment_variables"
						},
						"app": {
							"href": "%[1]s/v3/apps/%[2]s"
						}
					}
				}`, defaultServerURL, appGUID)))
			})

			It("fetches the right App", func() {
				Expect(appRepo.GetAppCallCount()).To(Equal(1))
				_, _, actualAppGUID := appRepo.GetAppArgsForCall(0)
				Expect(actualAppGUID).To(Equal(appGUID))
			})

			It("updates the k8s record via the repository", func() {
				Expect(appRepo.PatchAppEnvVarsCallCount()).To(Equal(1))
				_, _, message := appRepo.PatchAppEnvVarsArgsForCall(0)
				Expect(message.AppGUID).To(Equal(appGUID))
				Expect(message.SpaceGUID).To(Equal(spaceGUID))
				var value1 *string
				value2 := "VAL2"
				Expect(message.EnvironmentVariables).To(Equal(map[string]*string{
					key1: value1,
					key2: &value2,
				}))
			})
		})

		When("the validating env vars", func() {
			DescribeTable("returns response code",
				func(requestBody string, status int) {
					tableTestRecorder := httptest.NewRecorder()
					queuePatchRequest(appGUID, requestBody)
					router.ServeHTTP(tableTestRecorder, req)
					Expect(tableTestRecorder.Code).To(Equal(status))
				},
				Entry("contains a null value", `{ "var": { "key": null } }`, http.StatusOK),
				Entry("contains an int value", `{ "var": { "key": 9999 } }`, http.StatusOK),
				Entry("contains an float value", `{ "var": { "key": 9999.9 } }`, http.StatusOK),
				Entry("contains an bool value", `{ "var": { "key": true } }`, http.StatusOK),
				Entry("contains an string value", `{ "var": { "key": "string" } }`, http.StatusOK),
				Entry("contains a PORT key", `{ "var": { "PORT": 9000 } }`, http.StatusUnprocessableEntity),
				Entry("contains a VPORT key", `{ "var": { "VPORT": 9000 } }`, http.StatusOK),
				Entry("contains a PORTO key", `{ "var": { "PORTO": 9000 } }`, http.StatusOK),
				Entry("contains a VCAP_ key prefix", `{ "var": {"VCAP_POTATO":"foo" } }`, http.StatusUnprocessableEntity),
				Entry("contains a VMC_ key prefix", `{ "var": {"VMC_APPLE":"bar" } }`, http.StatusUnprocessableEntity),
			)
		})

		When("on the sad path and", func() {
			When("the request JSON is invalid", func() {
				BeforeEach(func() {
					queuePatchRequest(appGUID, `{`)
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

			When("the app cannot be found", func() {
				BeforeEach(func() {
					appRepo.GetAppReturns(repositories.AppRecord{}, repositories.NotFoundError{})
				})

				// TODO: should we return code 100004 instead?
				It("returns an error", func() {
					expectNotFoundError("App not found")
				})
			})

			When("there is some other error fetching the app", func() {
				BeforeEach(func() {
					appRepo.GetAppReturns(repositories.AppRecord{}, errors.New("unknown!"))
				})

				It("returns an error", func() {
					expectUnknownError()
				})
			})

			When("there is some error updating the app environment variables", func() {
				BeforeEach(func() {
					appRepo.PatchAppEnvVarsReturns(repositories.AppEnvVarsRecord{}, errors.New("unknown!"))
				})

				It("returns an error", func() {
					expectUnknownError()
				})
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
