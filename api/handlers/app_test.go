package handlers_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"

	apierrors "code.cloudfoundry.org/korifi/api/errors"
	. "code.cloudfoundry.org/korifi/api/handlers"
	"code.cloudfoundry.org/korifi/api/handlers/fake"
	"code.cloudfoundry.org/korifi/api/repositories"
	"code.cloudfoundry.org/korifi/tools"

	"github.com/go-http-utils/headers"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
)

const (
	appGUID     = "test-app-guid"
	appName     = "test-app"
	spaceGUID   = "test-space-guid"
	dropletGUID = "test-droplet-guid"
)

var _ = Describe("App", func() {
	var (
		appRepo       *fake.CFAppRepository
		dropletRepo   *fake.CFDropletRepository
		processRepo   *fake.CFProcessRepository
		routeRepo     *fake.CFRouteRepository
		processScaler *fake.AppProcessScaler
		domainRepo    *fake.CFDomainRepository
		spaceRepo     *fake.SpaceRepository
		req           *http.Request

		appRecord repositories.AppRecord
	)

	BeforeEach(func() {
		appRepo = new(fake.CFAppRepository)
		dropletRepo = new(fake.CFDropletRepository)
		processRepo = new(fake.CFProcessRepository)
		routeRepo = new(fake.CFRouteRepository)
		domainRepo = new(fake.CFDomainRepository)
		processScaler = new(fake.AppProcessScaler)
		spaceRepo = new(fake.SpaceRepository)
		decoderValidator, err := NewDefaultDecoderValidator()
		Expect(err).NotTo(HaveOccurred())

		apiHandler := NewApp(
			*serverURL,
			appRepo,
			dropletRepo,
			processRepo,
			routeRepo,
			domainRepo,
			spaceRepo,
			processScaler,
			decoderValidator,
		)

		appRecord = repositories.AppRecord{
			GUID:        appGUID,
			Name:        "test-app",
			SpaceGUID:   spaceGUID,
			State:       "STOPPED",
			DropletGUID: "test-droplet-guid",
			Lifecycle: repositories.Lifecycle{
				Type: "buildpack",
				Data: repositories.LifecycleData{
					Buildpacks: []string{},
				},
			},
			Annotations: map[string]string{
				AppRevisionKey:   "0",
				"annotation-key": "annotation-value",
			},
			Labels: map[string]string{
				"label-key": "label-value",
			},
		}
		appRepo.GetAppReturns(appRecord, nil)
		routerBuilder.LoadRoutes(apiHandler)
	})

	JustBeforeEach(func() {
		routerBuilder.Build().ServeHTTP(rr, req)
	})

	Describe("GET /v3/apps/:guid", func() {
		BeforeEach(func() {
			req = createHttpRequest("GET", "/v3/apps/"+appGUID, nil)
		})

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
                      "labels": {
                        "label-key": "label-value"
                      },
                      "annotations": {
						"korifi.cloudfoundry.org/app-rev": "0",
                        "annotation-key": "annotation-value"
                      }
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

		When("the app is not accessible", func() {
			BeforeEach(func() {
				appRepo.GetAppReturns(repositories.AppRecord{}, apierrors.NewForbiddenError(nil, repositories.AppResourceType))
			})

			It("returns an error", func() {
				expectNotFoundError("App not found")
			})
		})

		When("there is an error fetching the app", func() {
			BeforeEach(func() {
				appRepo.GetAppReturns(repositories.AppRecord{}, errors.New("unknown!"))
			})

			It("returns an error", func() {
				expectUnknownError()
			})
		})
	})

	Describe("POST /v3/apps", func() {
		requestBody := func(spaceGUID string) io.Reader {
			return strings.NewReader(`{
				"name": "` + appName + `",
				"relationships": {
					"space": {
						"data": {
							"guid": "` + spaceGUID + `"
						}
					}
				}
			}`)
		}

		BeforeEach(func() {
			appRepo.CreateAppReturns(appRecord, nil)
			req = createHttpRequest("POST", "/v3/apps", requestBody(spaceGUID))
		})

		It("returns status 201 Created", func() {
			Expect(rr.Code).To(Equal(http.StatusCreated))
		})

		It("passes authInfo from context to CreateApp", func() {
			Expect(appRepo.CreateAppCallCount()).To(Equal(1))
			_, actualAuthInfo, _ := appRepo.CreateAppArgsForCall(0)
			Expect(actualAuthInfo).To(Equal(authInfo))
		})

		It("returns the App in the response", func() {
			contentTypeHeader := rr.Header().Get("Content-Type")
			Expect(contentTypeHeader).To(Equal(jsonHeader))

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
                      "labels": {
                        "label-key": "label-value"
                      },
                      "annotations": {
						"korifi.cloudfoundry.org/app-rev": "0",
                        "annotation-key": "annotation-value"
                      }
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

		When("creating the app fails", func() {
			BeforeEach(func() {
				appRepo.CreateAppReturns(repositories.AppRecord{}, errors.New("create-app-err"))
			})

			It("returns an error", func() {
				expectUnknownError()
			})
		})

		When("the request body is invalid json", func() {
			BeforeEach(func() {
				req = createHttpRequest("POST", "/v3/apps", strings.NewReader(`{`))
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
				req = createHttpRequest("POST", "/v3/apps", strings.NewReader(`{"description" : "Invalid Request"}`))
			})

			It("returns an error", func() {
				expectUnprocessableEntityError(`invalid request body: json: unknown field "description"`)
			})
		})

		When("the request body is invalid with invalid app name", func() {
			BeforeEach(func() {
				req = createHttpRequest("POST", "/v3/apps", strings.NewReader(`{
					"name": 12345,
					"relationships": { "space": { "data": { "guid": "2f35885d-0c9d-4423-83ad-fd05066f8576" } } }
				}`))
			})

			It("returns an error", func() {
				expectUnprocessableEntityError("Name must be a string")
			})
		})

		When("the request body is invalid with invalid environment variable object", func() {
			BeforeEach(func() {
				req = createHttpRequest("POST", "/v3/apps", strings.NewReader(`{
					"name": "my_app",
					"environment_variables": [],
					"relationships": { "space": { "data": { "guid": "2f35885d-0c9d-4423-83ad-fd05066f8576" } } }
				}`))
			})

			It("returns an error", func() {
				expectUnprocessableEntityError("Environment_variables must be a map[string]string")
			})
		})

		When("the request body is invalid with missing required name field", func() {
			BeforeEach(func() {
				req = createHttpRequest("POST", "/v3/apps", strings.NewReader(`{
					"relationships": { "space": { "data": { "guid": "0c78dd5d-c723-4f2e-b168-df3c3e1d0806" } } }
				}`))
			})

			It("returns an error", func() {
				expectUnprocessableEntityError("Name is a required field")
			})
		})

		When("the request body is invalid with missing data within lifecycle", func() {
			BeforeEach(func() {
				req = createHttpRequest("POST", "/v3/apps", strings.NewReader(`{
					"name": "test-app",
					"lifecycle":{},
					"relationships": { "space": { "data": { "guid": "0c78dd5d-c723-4f2e-b168-df3c3e1d0806" } } }
				}`))
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
				spaceRepo.GetSpaceReturns(repositories.SpaceRecord{}, apierrors.NewNotFoundError(nil, repositories.SpaceResourceType))
				req = createHttpRequest("POST", "/v3/apps", requestBody("no-such-guid"))
			})

			It("returns an error", func() {
				expectUnprocessableEntityError("Invalid space. Ensure that the space exists and you have access to it.")
			})
		})

		When("the action errors", func() {
			BeforeEach(func() {
				appRepo.CreateAppReturns(repositories.AppRecord{}, errors.New("nope"))
			})

			It("returns an error", func() {
				expectUnknownError()
			})
		})
	})

	Describe("GET /v3/apps", func() {
		BeforeEach(func() {
			appRepo.ListAppsReturns([]repositories.AppRecord{
				{
					GUID:      "first-test-app-guid",
					Name:      "first-test-app",
					SpaceGUID: "test-space-guid",
					State:     "STOPPED",
					Annotations: map[string]string{
						AppRevisionKey: "0",
					},
					Lifecycle: repositories.Lifecycle{
						Type: "buildpack",
						Data: repositories.LifecycleData{
							Buildpacks: []string{},
						},
					},
				},
				{
					GUID:      "second-test-app-guid",
					Name:      "second-test-app",
					SpaceGUID: "test-space-guid",
					State:     "STOPPED",
					Annotations: map[string]string{
						AppRevisionKey: "0",
					},
					Lifecycle: repositories.Lifecycle{
						Type: "buildpack",
						Data: repositories.LifecycleData{
							Buildpacks: []string{},
						},
					},
				},
			}, nil)

			req = createHttpRequest("GET", "/v3/apps", nil)
		})

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
						  "annotations": {
						    "korifi.cloudfoundry.org/app-rev": "0"
						  }
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
						  "annotations": {
						    "korifi.cloudfoundry.org/app-rev": "0"
						  }
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

		It("invokes the repository with the provided auth info", func() {
			Expect(appRepo.ListAppsCallCount()).To(Equal(1))
			_, actualAuthInfo, _ := appRepo.ListAppsArgsForCall(0)
			Expect(actualAuthInfo).To(Equal(authInfo))
		})

		When("filtering query params are provided", func() {
			BeforeEach(func() {
				req = createHttpRequest("GET", "/v3/apps?names=app1,app2&space_guids=space1,space2", nil)
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

		Describe("Order results", func() {
			type res struct {
				GUID string `json:"guid"`
			}
			type resList struct {
				Resources []res `json:"resources"`
			}

			BeforeEach(func() {
				appRepo.ListAppsReturns([]repositories.AppRecord{
					{
						GUID:      "1",
						Name:      "first-test-app",
						State:     "STOPPED",
						CreatedAt: "2023-01-17T14:58:32Z",
						UpdatedAt: "2023-01-18T14:58:32Z",
					},
					{
						GUID:      "2",
						Name:      "second-test-app",
						State:     "BROKEN",
						CreatedAt: "2023-01-17T14:57:32Z",
						UpdatedAt: "2023-01-19T14:57:32Z",
					},
					{
						GUID:      "3",
						Name:      "third-test-app",
						State:     "STARTED",
						CreatedAt: "2023-01-16T14:57:32Z",
						UpdatedAt: "2023-01-20:57:32Z",
					},
					{
						GUID:      "4",
						Name:      "fourth-test-app",
						State:     "FIXED",
						CreatedAt: "2023-01-17T13:57:32Z",
						UpdatedAt: "2023-01-19T14:58:32Z",
					},
				}, nil)
			})

			DescribeTable("ordering results", func(orderBy string, expectedOrder ...string) {
				req = createHttpRequest("GET", "/v3/apps?order_by="+orderBy, nil)
				rr = httptest.NewRecorder()
				routerBuilder.Build().ServeHTTP(rr, req)
				var respList resList
				err := json.Unmarshal(rr.Body.Bytes(), &respList)
				Expect(err).NotTo(HaveOccurred())
				expectedList := make([]res, len(expectedOrder))
				for i := range expectedOrder {
					expectedList[i] = res{GUID: expectedOrder[i]}
				}
				Expect(respList.Resources).To(Equal(expectedList))
			},
				Entry("created_at ASC", "created_at", "3", "4", "2", "1"),
				Entry("created_at DESC", "-created_at", "1", "2", "4", "3"),
				Entry("updated_at ASC", "updated_at", "1", "2", "4", "3"),
				Entry("updated_at DESC", "-updated_at", "3", "4", "2", "1"),
				Entry("name ASC", "name", "1", "4", "2", "3"),
				Entry("name DESC", "-name", "3", "2", "4", "1"),
				Entry("state ASC", "state", "2", "4", "3", "1"),
				Entry("state DESC", "-state", "1", "3", "4", "2"),
			)

			When("order_by is not a valid field", func() {
				BeforeEach(func() {
					req = createHttpRequest("GET", "/v3/apps?order_by=not_valid", nil)
				})

				It("returns an Unknown key error", func() {
					expectUnknownKeyError("The query parameter is invalid: Order by can only be: 'created_at', 'updated_at', 'name', 'state'")
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

		When("there is an error fetching apps", func() {
			BeforeEach(func() {
				appRepo.ListAppsReturns([]repositories.AppRecord{}, errors.New("unknown!"))
			})

			It("returns an error", func() {
				expectUnknownError()
			})
		})

		When("invalid query parameters are provided", func() {
			BeforeEach(func() {
				req = createHttpRequest("GET", "/v3/apps?foo=bar", nil)
			})

			It("returns an Unknown key error", func() {
				expectUnknownKeyError("The query parameter is invalid: Valid parameters are: 'names, guids, space_guids, order_by'")
			})
		})
	})

	Describe("PATCH /v3/apps/:guid", func() {
		BeforeEach(func() {
			appRepo.PatchAppMetadataReturns(appRecord, nil)
			req = createHttpRequest("PATCH", "/v3/apps/"+appGUID, strings.NewReader(`{
				  "metadata": {
					"labels": {
					  "env": "production",
					  "foo.example.com/my-identifier": "aruba"
					},
					"annotations": {
					  "hello": "there",
					  "foo.example.com/lorem-ipsum": "Lorem ipsum."
					}
				  }
				}`))
		})

		It("returns status 200 OK", func() {
			Expect(rr.Code).To(Equal(http.StatusOK), "Matching HTTP response code:")
		})

		It("patches the app with the new labels and annotations", func() {
			Expect(appRepo.PatchAppMetadataCallCount()).To(Equal(1))
			_, _, msg := appRepo.PatchAppMetadataArgsForCall(0)
			Expect(msg.AppGUID).To(Equal(appGUID))
			Expect(msg.SpaceGUID).To(Equal(spaceGUID))
			Expect(msg.Annotations).To(HaveKeyWithValue("hello", PointTo(Equal("there"))))
			Expect(msg.Annotations).To(HaveKeyWithValue("foo.example.com/lorem-ipsum", PointTo(Equal("Lorem ipsum."))))
			Expect(msg.Labels).To(HaveKeyWithValue("env", PointTo(Equal("production"))))
			Expect(msg.Labels).To(HaveKeyWithValue("foo.example.com/my-identifier", PointTo(Equal("aruba"))))
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
                      "labels": {
                        "label-key": "label-value"
                      },
                      "annotations": {
						"korifi.cloudfoundry.org/app-rev": "0",
                        "annotation-key": "annotation-value"
                      }
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

		When("the user doesn't have permission to get the App", func() {
			BeforeEach(func() {
				appRepo.GetAppReturns(repositories.AppRecord{}, apierrors.NewForbiddenError(nil, repositories.AppResourceType))
			})

			It("returns a not found error", func() {
				expectNotFoundError("App not found")
			})

			It("does not call patch", func() {
				Expect(appRepo.PatchAppMetadataCallCount()).To(Equal(0))
			})
		})

		When("fetching the App errors", func() {
			BeforeEach(func() {
				appRepo.GetAppReturns(repositories.AppRecord{}, errors.New("boom"))
			})

			It("returns an error", func() {
				expectUnknownError()
			})

			It("does not call patch", func() {
				Expect(appRepo.PatchAppMetadataCallCount()).To(Equal(0))
			})
		})

		When("patching the App errors", func() {
			BeforeEach(func() {
				appRepo.PatchAppMetadataReturns(repositories.AppRecord{}, errors.New("boom"))
			})

			It("returns an error", func() {
				expectUnknownError()
			})
		})

		When("a label is invalid", func() {
			When("the prefix is cloudfoundry.org", func() {
				BeforeEach(func() {
					req = createHttpRequest("PATCH", "/v3/apps/"+appGUID, strings.NewReader(`{
					  "metadata": {
						"labels": {
						  "cloudfoundry.org/test": "production"
					    }
        		      }
					}`))
				})

				It("returns an unprocessable entity error", func() {
					expectUnprocessableEntityError(`Labels and annotations cannot begin with "cloudfoundry.org" or its subdomains`)
				})
			})

			When("the prefix is a subdomain of cloudfoundry.org", func() {
				BeforeEach(func() {
					req = createHttpRequest("PATCH", "/v3/apps/"+appGUID, strings.NewReader(`{
					  "metadata": {
						"labels": {
						  "korifi.cloudfoundry.org/test": "production"
					    }
    		          }
					}`))
				})

				It("returns an unprocessable entity error", func() {
					expectUnprocessableEntityError(`Labels and annotations cannot begin with "cloudfoundry.org" or its subdomains`)
				})
			})
		})

		When("an annotation is invalid", func() {
			When("the prefix is cloudfoundry.org", func() {
				BeforeEach(func() {
					req = createHttpRequest("PATCH", "/v3/apps/"+appGUID, strings.NewReader(`{
					  "metadata": {
						"annotations": {
						  "cloudfoundry.org/test": "there"
						}
					  }
					}`))
				})

				It("returns an unprocessable entity error", func() {
					expectUnprocessableEntityError(`Labels and annotations cannot begin with "cloudfoundry.org" or its subdomains`)
				})

				When("the prefix is a subdomain of cloudfoundry.org", func() {
					BeforeEach(func() {
						req = createHttpRequest("PATCH", "/v3/apps/"+appGUID, strings.NewReader(`{
						  "metadata": {
							"annotations": {
							  "korifi.cloudfoundry.org/test": "there"
							}
						  }
						}`))
					})

					It("returns an unprocessable entity error", func() {
						expectUnprocessableEntityError(`Labels and annotations cannot begin with "cloudfoundry.org" or its subdomains`)
					})
				})
			})
		})
	})

	Describe("PATCH /v3/apps/:guid/relationships/current_droplet", func() {
		var droplet repositories.DropletRecord

		BeforeEach(func() {
			droplet = repositories.DropletRecord{GUID: dropletGUID, AppGUID: appGUID}

			dropletRepo.GetDropletReturns(droplet, nil)
			appRepo.SetCurrentDropletReturns(repositories.CurrentDropletRecord{
				AppGUID:     appGUID,
				DropletGUID: dropletGUID,
			}, nil)

			req = createHttpRequest("PATCH", "/v3/apps/"+appGUID+"/relationships/current_droplet", strings.NewReader(`
					{ "data": { "guid": "`+dropletGUID+`" } }
                `))
		})

		itDoesntSetTheCurrentDroplet := func() {
			It("doesn't set the current droplet on the app", func() {
				Expect(appRepo.SetCurrentDropletCallCount()).To(Equal(0))
			})
		}

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

		When("setting the current droplet fails", func() {
			BeforeEach(func() {
				appRepo.SetCurrentDropletReturns(repositories.CurrentDropletRecord{}, errors.New("set-droplet-failed"))
			})

			It("returns a not authenticated error", func() {
				expectUnknownError()
			})
		})

		When("getting the app fails", func() {
			BeforeEach(func() {
				appRepo.GetAppReturns(repositories.AppRecord{}, errors.New("get-app-failed"))
			})

			It("returns an error", func() {
				expectUnknownError()
			})
			itDoesntSetTheCurrentDroplet()
		})

		When("the App cannot be accessed", func() {
			BeforeEach(func() {
				appRepo.GetAppReturns(repositories.AppRecord{}, apierrors.NewForbiddenError(nil, repositories.AppResourceType))
			})

			It("returns an error", func() {
				expectNotFoundError("App not found")
			})
			itDoesntSetTheCurrentDroplet()
		})

		When("the Droplet doesn't exist", func() {
			BeforeEach(func() {
				dropletRepo.GetDropletReturns(repositories.DropletRecord{}, apierrors.NewNotFoundError(nil, repositories.DropletResourceType))
			})

			It("returns an error", func() {
				expectUnprocessableEntityError("Unable to assign current droplet. Ensure the droplet exists and belongs to this app.")
			})
			itDoesntSetTheCurrentDroplet()
		})

		When("the Droplet isn't accessible to the user", func() {
			BeforeEach(func() {
				dropletRepo.GetDropletReturns(repositories.DropletRecord{}, apierrors.NewForbiddenError(nil, repositories.DropletResourceType))
			})

			It("returns an error", func() {
				expectUnprocessableEntityError("Unable to assign current droplet. Ensure the droplet exists and belongs to this app.")
			})
			itDoesntSetTheCurrentDroplet()
		})

		When("getting the droplet fails", func() {
			BeforeEach(func() {
				dropletRepo.GetDropletReturns(repositories.DropletRecord{}, errors.New("get-droplet-failed"))
			})

			It("returns an error", func() {
				expectUnknownError()
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
				req = createHttpRequest("PATCH", "/v3/apps/"+appGUID+"/relationships/current_droplet", strings.NewReader(`
					{ "data": {  } }
                `))
			})

			It("returns an error", func() {
				expectUnprocessableEntityError("GUID is a required field")
			})
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

	Describe("POST /v3/apps/:guid/actions/start", func() {
		BeforeEach(func() {
			updatedAppRecord := appRecord
			updatedAppRecord.State = "STARTED"
			appRepo.SetAppDesiredStateReturns(updatedAppRecord, nil)

			req = createHttpRequest("POST", "/v3/apps/"+appGUID+"/actions/start", nil)
		})

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
                      "labels": {
					    "label-key": "label-value"
					  },
                      "annotations": {
					    "annotation-key": "annotation-value",
					    "korifi.cloudfoundry.org/app-rev": "0"
					  }
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

		When("getting the app is forbidden", func() {
			BeforeEach(func() {
				appRepo.GetAppReturns(repositories.AppRecord{}, apierrors.NewForbiddenError(nil, repositories.AppResourceType))
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
				appRecord.DropletGUID = ""
				appRepo.GetAppReturns(appRecord, nil)
			})

			It("returns an error", func() {
				expectUnprocessableEntityError(`Assign a droplet before starting this app.`)
			})
		})

		When("there is an error updating app desiredState", func() {
			BeforeEach(func() {
				appRepo.SetAppDesiredStateReturns(repositories.AppRecord{}, errors.New("unknown!"))
			})

			It("returns an error", func() {
				expectUnknownError()
			})
		})
	})

	Describe("POST /v3/apps/:guid/actions/stop", func() {
		BeforeEach(func() {
			updatedAppRecord := appRecord
			updatedAppRecord.State = "STOPPED"
			appRepo.SetAppDesiredStateReturns(updatedAppRecord, nil)
			appRepo.PatchAppMetadataReturns(updatedAppRecord, nil)

			req = createHttpRequest("POST", "/v3/apps/"+appGUID+"/actions/stop", nil)
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
						"labels": {
						  "label-key": "label-value"
						},
						"annotations": {
					      "annotation-key": "annotation-value",
						  "korifi.cloudfoundry.org/app-rev": "0"
						}
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

		It("bumps the app revision annotation", func() {
			Expect(appRepo.PatchAppMetadataCallCount()).To(Equal(1))
			_, _, actualPatchMsg := appRepo.PatchAppMetadataArgsForCall(0)
			Expect(actualPatchMsg.Annotations).To(HaveKeyWithValue(AppRevisionKey, tools.PtrTo("1")))
		})

		When("bumping the app revision annotation fails", func() {
			BeforeEach(func() {
				appRepo.PatchAppMetadataReturns(repositories.AppRecord{}, errors.New("patch-app-rev-err"))
			})

			It("returns an error", func() {
				expectUnprocessableEntityError("failed to update app revision")
			})
		})

		When("the app revision cannot be parsed", func() {
			BeforeEach(func() {
				appRecord.Annotations[AppRevisionKey] = "nan"
			})

			It("returns an error", func() {
				expectUnprocessableEntityError("failed to parse app revision")
			})
		})

		When("fetching the app is forbidden", func() {
			BeforeEach(func() {
				appRepo.GetAppReturns(repositories.AppRecord{}, apierrors.NewForbiddenError(nil, "App"))
			})

			It("returns a not found error", func() {
				expectNotFoundError("App not found")
			})
		})

		When("there is an unknown error fetching the app", func() {
			BeforeEach(func() {
				appRepo.GetAppReturns(repositories.AppRecord{}, errors.New("unknown!"))
			})

			It("returns an error", func() {
				expectUnknownError()
			})
		})

		When("there is an unknown error updating app desiredState", func() {
			BeforeEach(func() {
				appRepo.SetAppDesiredStateReturns(repositories.AppRecord{}, errors.New("unknown!"))
			})

			It("returns an error", func() {
				expectUnknownError()
			})
		})
	})

	Describe("GET /v3/apps/:guid/processes", func() {
		var (
			process1Record repositories.ProcessRecord
			process2Record repositories.ProcessRecord
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
				},
				Labels:      map[string]string{},
				Annotations: map[string]string{},
				CreatedAt:   "2016-03-23T18:48:22Z",
				UpdatedAt:   "2016-03-23T18:48:42Z",
			}

			process1Record = processRecord

			process2Record = processRecord
			process2Record.GUID = "process-2-guid"
			process2Record.Type = "worker"
			process2Record.DesiredInstances = 1
			process2Record.HealthCheck.Type = "process"

			processRepo.ListProcessesReturns([]repositories.ProcessRecord{
				process1Record,
				process2Record,
			}, nil)

			req = createHttpRequest("GET", "/v3/apps/"+appGUID+"/processes", nil)
		})

		It("returns status 200 OK", func() {
			Expect(rr.Code).To(Equal(http.StatusOK), "Matching HTTP response code:")
		})

		It("returns the processes", func() {
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

		When("the app cannot be accessed", func() {
			BeforeEach(func() {
				appRepo.GetAppReturns(repositories.AppRecord{}, apierrors.NewForbiddenError(nil, repositories.AppResourceType))
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

	Describe("GET /v3/apps/:guid/processes/{type}", func() {
		BeforeEach(func() {
			processRepo.GetProcessByAppTypeAndSpaceReturns(repositories.ProcessRecord{
				GUID:             "process-1-guid",
				SpaceGUID:        spaceGUID,
				AppGUID:          appGUID,
				Type:             "web",
				Command:          "bundle exec rackup config.ru -p $PORT -o 0.0.0.0",
				DesiredInstances: 1,
				MemoryMB:         256,
				DiskQuotaMB:      1024,
				Ports:            []int32{8080},
				HealthCheck: repositories.HealthCheck{
					Type: "port",
				},
				Labels:      map[string]string{},
				Annotations: map[string]string{},
				CreatedAt:   "1906-04-18T13:12:00Z",
				UpdatedAt:   "1906-04-18T13:12:01Z",
			}, nil)

			req = createHttpRequest("GET", "/v3/apps/"+appGUID+"/processes/web", nil)
		})

		It("returns status 200 OK", func() {
			Expect(rr.Code).To(Equal(http.StatusOK))
		})

		It("passes the authorization.Info to the process repository", func() {
			Expect(processRepo.GetProcessByAppTypeAndSpaceCallCount()).To(Equal(1))
			_, actualAuthInfo, _, _, _ := processRepo.GetProcessByAppTypeAndSpaceArgsForCall(0)
			Expect(actualAuthInfo).To(Equal(authInfo))
		})

		It("returns a process", func() {
			contentTypeHeader := rr.Header().Get("Content-Type")
			Expect(contentTypeHeader).To(Equal(jsonHeader), "Matching Content-Type header:")

			Expect(rr.Body.String()).To(MatchJSON(`{
					"guid": "process-1-guid",
					"created_at": "1906-04-18T13:12:00Z",
					"updated_at": "1906-04-18T13:12:01Z",
					"type": "web",
					"command": "bundle exec rackup config.ru -p $PORT -o 0.0.0.0",
					"instances": 1,
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
						  "href": "https://api.example.org/v3/processes/process-1-guid"
					   },
					   "scale": {
						  "href": "https://api.example.org/v3/processes/process-1-guid/actions/scale",
						  "method": "POST"
					   },
					   "app": {
						  "href": "https://api.example.org/v3/apps/` + appGUID + `"
					   },
					   "space": {
						  "href": "https://api.example.org/v3/spaces/` + spaceGUID + `"
					   },
					   "stats": {
						  "href": "https://api.example.org/v3/processes/process-1-guid/stats"
					   }
					}
				 }`))
		})

		When("the user lacks access in the app namespace", func() {
			BeforeEach(func() {
				appRepo.GetAppReturns(repositories.AppRecord{}, apierrors.NewForbiddenError(errors.New("Forbidden"), repositories.AppResourceType))
			})

			It("returns an not found error", func() {
				expectNotFoundError("App not found")
			})
		})

		When("getting the app fails", func() {
			BeforeEach(func() {
				appRepo.GetAppReturns(repositories.AppRecord{}, errors.New("get-app"))
			})

			It("returns an error", func() {
				expectUnknownError()
			})
		})

		When("there is an error fetching processes", func() {
			BeforeEach(func() {
				processRepo.GetProcessByAppTypeAndSpaceReturns(repositories.ProcessRecord{}, errors.New("some-error"))
			})

			It("return a process unknown error", func() {
				expectUnknownError()
			})
		})
	})

	Describe("the POST /v3/apps/:guid/process/:processType/actions/scale endpoint", func() {
		BeforeEach(func() {
			processScaler.ScaleAppProcessReturns(repositories.ProcessRecord{
				GUID:             "process-1-guid",
				SpaceGUID:        spaceGUID,
				AppGUID:          appGUID,
				Type:             "web",
				Command:          "bundle exec rackup config.ru -p $PORT -o 0.0.0.0",
				DesiredInstances: 5,
				MemoryMB:         256,
				DiskQuotaMB:      1024,
				Ports:            []int32{8080},
				HealthCheck: repositories.HealthCheck{
					Type: "port",
				},
				Labels:      map[string]string{},
				Annotations: map[string]string{},
				CreatedAt:   "1906-04-18T13:12:00Z",
				UpdatedAt:   "1906-04-18T13:12:01Z",
			}, nil)

			req = createHttpRequest("POST", "/v3/apps/"+appGUID+"/processes/web/actions/scale", strings.NewReader(fmt.Sprintf(`{
				"instances": %d,
				"memory_in_mb": %d,
				"disk_in_mb": %d
			}`, 5, 256, 1024)))
		})

		It("returns status 200 OK", func() {
			Expect(rr.Code).To(Equal(http.StatusOK), "Matching HTTP response code:")
		})

		It("returns the scaled process", func() {
			contentTypeHeader := rr.Header().Get("Content-Type")
			Expect(contentTypeHeader).To(Equal(jsonHeader), "Matching Content-Type header:")

			Expect(rr.Body.String()).To(MatchJSON(`{
						"guid": "process-1-guid",
						"created_at": "1906-04-18T13:12:00Z",
						"updated_at": "1906-04-18T13:12:01Z",
						"type": "web",
						"command": "bundle exec rackup config.ru -p $PORT -o 0.0.0.0",
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
							  "href": "https://api.example.org/v3/processes/process-1-guid"
						   },
						   "scale": {
							  "href": "https://api.example.org/v3/processes/process-1-guid/actions/scale",
							  "method": "POST"
						   },
						   "app": {
							  "href": "https://api.example.org/v3/apps/` + appGUID + `"
						   },
						   "space": {
							  "href": "https://api.example.org/v3/spaces/` + spaceGUID + `"
						   },
						   "stats": {
							  "href": "https://api.example.org/v3/processes/process-1-guid/stats"
						   }
						}
				 }`))
		})

		When("only some fields are set", func() {
			BeforeEach(func() {
				req = createHttpRequest("POST", "/v3/apps/"+appGUID+"/processes/web/actions/scale", strings.NewReader(fmt.Sprintf(`{
						"memory_in_mb": %d
					}`, 1024)))
			})

			It("invokes the scale function with only a subset of the scale fields", func() {
				Expect(processScaler.ScaleAppProcessCallCount()).To(Equal(1), "did not call scaleProcess just once")
				_, _, _, _, invokedProcessScale := processScaler.ScaleAppProcessArgsForCall(0)
				Expect(invokedProcessScale.Instances).To(BeNil())
				Expect(invokedProcessScale.DiskMB).To(BeNil())
				Expect(*invokedProcessScale.MemoryMB).To(BeNumerically("==", 1024))
			})
		})

		When("the request JSON is invalid", func() {
			BeforeEach(func() {
				req = createHttpRequest("POST", "/v3/apps/"+appGUID+"/processes/web/actions/scale", strings.NewReader(`}`))
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

		When("there is an error scaling the app", func() {
			BeforeEach(func() {
				processScaler.ScaleAppProcessReturns(repositories.ProcessRecord{}, errors.New("unknown!"))
			})

			It("returns an error", func() {
				expectUnknownError()
			})
		})

		DescribeTable("request body validation",
			func(requestBody string, status int) {
				tableTestRecorder := httptest.NewRecorder()
				req = createHttpRequest("POST", "/v3/apps/"+appGUID+"/processes/web/actions/scale", strings.NewReader(requestBody))
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

	Describe("GET /v3/apps/:guid/routes", func() {
		BeforeEach(func() {
			routeRepo.ListRoutesForAppReturns([]repositories.RouteRecord{
				{
					GUID:      "test-route-guid",
					SpaceGUID: "test-space-guid",
					Domain: repositories.DomainRecord{
						GUID: "test-domain-guid",
					},

					Host:         "test-route-host",
					Path:         "/some_path",
					Protocol:     "http",
					Destinations: nil,
					Labels:       nil,
					Annotations:  nil,
					CreatedAt:    "2019-05-10T17:17:48Z",
					UpdatedAt:    "2019-05-10T17:17:48Z",
				},
			}, nil)

			domainRepo.GetDomainReturns(repositories.DomainRecord{
				GUID: "test-domain-guid",
				Name: "example.org",
			}, nil)

			req = createHttpRequest("GET", "/v3/apps/"+appGUID+"/routes", nil)
		})

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

		It("returns status 200 OK", func() {
			Expect(rr.Code).To(Equal(http.StatusOK), "Matching HTTP response code:")
		})

		It("returns Content-Type as JSON in header", func() {
			contentTypeHeader := rr.Header().Get("Content-Type")
			Expect(contentTypeHeader).To(Equal(jsonHeader), "Matching Content-Type header:")
		})

		It("returns the Pagination Data and App Resources in the response", func() {
			Expect(rr.Body.String()).To(MatchJSON(`{
						"pagination": {
							"total_results": 1,
							"total_pages": 1,
							"first": {
								"href": "https://api.example.org/v3/apps/test-app-guid/routes"
							},
							"last": {
								"href": "https://api.example.org/v3/apps/test-app-guid/routes"
							},
							"next": null,
							"previous": null
						},
						"resources": [
							{
								"guid": "test-route-guid",
								"port": null,
								"path": "/some_path",
								"protocol": "http",
								"host": "test-route-host",
								"url": "test-route-host.example.org/some_path",
								"created_at": "2019-05-10T17:17:48Z",
								"updated_at": "2019-05-10T17:17:48Z",
								"destinations": [],
								"relationships": {
									"space": {
										"data": {
											"guid": "test-space-guid"
										}
									},
									"domain": {
										"data": {
											"guid": "test-domain-guid"
										}
									}
								},
								"metadata": {
									"labels": {},
									"annotations": {}
								},
								"links": {
									"self":{
										"href": "https://api.example.org/v3/routes/test-route-guid"
									},
									"space":{
										"href": "https://api.example.org/v3/spaces/test-space-guid"
									},
									"domain":{
										"href": "https://api.example.org/v3/domains/test-domain-guid"
									},
									"destinations":{
										"href": "https://api.example.org/v3/routes/test-route-guid/destinations"
									}
								}
							}
						]
					}`))
		})

		When("the app cannot be accessed", func() {
			BeforeEach(func() {
				appRepo.GetAppReturns(repositories.AppRecord{}, apierrors.NewForbiddenError(nil, repositories.AppResourceType))
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

	Describe("GET /v3/apps/:guid/droplets/current", func() {
		BeforeEach(func() {
			dropletRepo.GetDropletReturns(repositories.DropletRecord{
				GUID:      dropletGUID,
				State:     "STAGED",
				CreatedAt: "2019-05-10T17:17:48Z",
				UpdatedAt: "2019-05-10T17:17:48Z",
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
				PackageGUID: "test-package-guid",
			}, nil)

			req = createHttpRequest("GET", "/v3/apps/"+appGUID+"/droplets/current", nil)
		})

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
					  "guid": "` + dropletGUID + `",
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
					  "created_at": "2019-05-10T17:17:48Z",
					  "updated_at": "2019-05-10T17:17:48Z",
					  "relationships": {
						"app": {
						  "data": {
							"guid": "` + appGUID + `"
						  }
						}
					  },
					  "links": {
						"self": {
						  "href": "` + defaultServerURI("/v3/droplets/", dropletGUID) + `"
						},
						"package": {
						  "href": "` + defaultServerURI("/v3/packages/", "test-package-guid") + `"
						},
						"app": {
						  "href": "` + defaultServerURI("/v3/apps/", appGUID) + `"
						},
						"assign_current_droplet": {
						  "href": "` + defaultServerURI("/v3/apps/", appGUID, "/relationships/current_droplet") + `",
						  "method": "PATCH"
						  },
						"download": null
					  },
					  "metadata": {
						"labels": {},
						"annotations": {}
					  }
					}`))
		})

		It("fetches the correct app", func() {
			Expect(appRepo.GetAppCallCount()).To(Equal(1))
			_, _, actualAppGUID := appRepo.GetAppArgsForCall(0)
			Expect(actualAppGUID).To(Equal(appGUID))
		})

		It("fetches the correct droplet", func() {
			Expect(dropletRepo.GetDropletCallCount()).To(Equal(1))
			_, _, actualDropletGUID := dropletRepo.GetDropletArgsForCall(0)
			Expect(actualDropletGUID).To(Equal(dropletGUID))
		})

		When("the app is not accessible", func() {
			BeforeEach(func() {
				appRepo.GetAppReturns(repositories.AppRecord{}, apierrors.NewForbiddenError(nil, repositories.AppResourceType))
			})

			It("returns an error", func() {
				expectNotFoundError("App not found")
			})
		})

		When("getting the app fails", func() {
			BeforeEach(func() {
				appRepo.GetAppReturns(repositories.AppRecord{}, errors.New("get-app"))
			})

			It("returns an error", func() {
				expectUnknownError()
			})
		})

		When("the app doesn't have a current droplet assigned", func() {
			BeforeEach(func() {
				appRepo.GetAppReturns(repositories.AppRecord{
					GUID:        appGUID,
					SpaceGUID:   spaceGUID,
					DropletGUID: "",
				}, nil)
			})

			It("returns a NotFound error with code 10010 (that is ignored by the cf cli)", func() {
				Expect(rr).To(HaveHTTPStatus(http.StatusNotFound))
				Expect(rr).To(HaveHTTPHeaderWithValue(headers.ContentType, jsonHeader))
				var bodyJSON map[string]interface{}
				Expect(json.Unmarshal(rr.Body.Bytes(), &bodyJSON)).To(Succeed())
				Expect(bodyJSON).To(HaveKey("errors"))
				Expect(bodyJSON["errors"]).To(HaveLen(1))
				Expect(bodyJSON["errors"]).To(ConsistOf(
					MatchAllKeys(Keys{
						"code":   BeEquivalentTo(10010),
						"title":  Equal("CF-ResourceNotFound"),
						"detail": Equal("Droplet not found"),
					}),
				))
			})
		})

		When("the user cannot access the droplet", func() {
			BeforeEach(func() {
				dropletRepo.GetDropletReturns(repositories.DropletRecord{}, apierrors.NewForbiddenError(nil, repositories.DropletResourceType))
			})

			It("returns a NotFound error with code 10010 (that is ignored by the cf cli)", func() {
				Expect(rr).To(HaveHTTPStatus(http.StatusNotFound))
				Expect(rr).To(HaveHTTPHeaderWithValue(headers.ContentType, jsonHeader))
				var bodyJSON map[string]interface{}
				Expect(json.Unmarshal(rr.Body.Bytes(), &bodyJSON)).To(Succeed())
				Expect(bodyJSON).To(HaveKey("errors"))
				Expect(bodyJSON["errors"]).To(HaveLen(1))
				Expect(bodyJSON["errors"]).To(ConsistOf(
					MatchAllKeys(Keys{
						"code":   BeEquivalentTo(10010),
						"title":  Equal("CF-ResourceNotFound"),
						"detail": Equal("Droplet not found"),
					}),
				))
			})
		})

		When("getting the droplet fails", func() {
			BeforeEach(func() {
				dropletRepo.GetDropletReturns(repositories.DropletRecord{}, errors.New("get-droplet"))
			})

			It("returns an error", func() {
				expectUnknownError()
			})
		})
	})

	Describe("GET /v3/apps/:guid/actions/restart", func() {
		BeforeEach(func() {
			updatedAppRecord := appRecord
			updatedAppRecord.State = "STARTED"
			appRepo.SetAppDesiredStateReturns(updatedAppRecord, nil)

			req = createHttpRequest("POST", "/v3/apps/"+appGUID+"/actions/restart", nil)
		})

		It("responds with a 200 code", func() {
			Expect(rr.Code).To(Equal(200))
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

			Expect(rr.Body.String()).To(MatchJSON(`{
                    "guid": "test-app-guid",
                    "created_at": "",
                    "updated_at": "",
                    "name": "test-app",
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
                          "guid": "test-space-guid"
                        }
                      }
                    },
                    "metadata": {
                      "labels": {
                        "label-key": "label-value"
                      },
                      "annotations": {
						"korifi.cloudfoundry.org/app-rev": "0",
                        "annotation-key": "annotation-value"
                      }
                    },
                    "links": {
                      "self": {
                        "href": "https://api.example.org/v3/apps/test-app-guid"
                      },
                      "environment_variables": {
                        "href": "https://api.example.org/v3/apps/test-app-guid/environment_variables"
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
                      "current_droplet": {
                        "href": "https://api.example.org/v3/apps/test-app-guid/droplets/current"
                      },
                      "droplets": {
                        "href": "https://api.example.org/v3/apps/test-app-guid/droplets"
                      },
                      "tasks": {
                        "href": "https://api.example.org/v3/apps/test-app-guid/tasks"
                      },
                      "start": {
                        "href": "https://api.example.org/v3/apps/test-app-guid/actions/start",
                        "method": "POST"
                      },
                      "stop": {
                        "href": "https://api.example.org/v3/apps/test-app-guid/actions/stop",
                        "method": "POST"
                      },
                      "revisions": {
                        "href": "https://api.example.org/v3/apps/test-app-guid/revisions"
                      },
                      "deployed_revisions": {
                        "href": "https://api.example.org/v3/apps/test-app-guid/revisions/deployed"
                      },
                      "features": {
                        "href": "https://api.example.org/v3/apps/test-app-guid/features"
                      }
                    }
                }`))
		})

		When("no permissions to get the app", func() {
			BeforeEach(func() {
				appRepo.GetAppReturns(repositories.AppRecord{}, apierrors.NewForbiddenError(nil, repositories.AppResourceType))
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
				appRecord.DropletGUID = ""
				appRepo.GetAppReturns(appRecord, nil)
				appRepo.SetAppDesiredStateReturns(appRecord, nil)
			})

			It("returns an error", func() {
				expectUnprocessableEntityError(`Assign a droplet before starting this app.`)
			})
		})

		When("stopping the app fails", func() {
			BeforeEach(func() {
				appRepo.SetAppDesiredStateReturnsOnCall(0, repositories.AppRecord{}, errors.New("stop-app"))
			})

			It("returns an error", func() {
				expectUnknownError()
			})
		})

		When("starting the app fails", func() {
			BeforeEach(func() {
				appRepo.SetAppDesiredStateReturnsOnCall(1, repositories.AppRecord{}, errors.New("start-app"))
			})

			It("returns a forbidden error", func() {
				expectUnknownError()
			})
		})

		When("the app is in STOPPED state", func() {
			BeforeEach(func() {
				appRecord.State = "STOPPED"
				appRepo.GetAppReturns(appRecord, nil)
			})

			It("responds with a 200 code", func() {
				Expect(rr.Code).To(Equal(200))
			})

			It("returns the app in the response with a state of STARTED", func() {
				Expect(rr.Body.String()).To(ContainSubstring(`"state":"STARTED"`))
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

	Describe("DELETE /v3/apps/:guid", func() {
		BeforeEach(func() {
			req = createHttpRequest("DELETE", "/v3/apps/"+appGUID, nil)
		})

		It("responds with a 202 accepted response", func() {
			Expect(rr.Code).To(Equal(http.StatusAccepted))
		})

		It("responds with a job URL in a location header", func() {
			locationHeader := rr.Header().Get("Location")
			Expect(locationHeader).To(Equal("https://api.example.org/v3/jobs/app.delete~"+appGUID), "Matching Location header")
		})

		It("fetches the right app", func() {
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

		When("fetching the app errors", func() {
			BeforeEach(func() {
				appRepo.GetAppReturns(repositories.AppRecord{}, errors.New("boom"))
			})

			It("returns an error", func() {
				expectUnknownError()
			})
		})

		When("deleting the app errors", func() {
			BeforeEach(func() {
				appRepo.DeleteAppReturns(errors.New("boom"))
			})

			It("returns an error", func() {
				expectUnknownError()
			})
		})
	})

	Describe("GET /v3/apps/:guid/env", func() {
		BeforeEach(func() {
			appRepo.GetAppEnvReturns(repositories.AppEnvRecord{
				AppGUID:              appGUID,
				SpaceGUID:            spaceGUID,
				EnvironmentVariables: map[string]string{"VAR": "VAL"},
				SystemEnv:            map[string]interface{}{},
			}, nil)

			req = createHttpRequest("GET", "/v3/apps/"+appGUID+"/env", nil)
		})

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

		When("there is an error fetching the app env", func() {
			BeforeEach(func() {
				appRepo.GetAppEnvReturns(repositories.AppEnvRecord{}, errors.New("unknown!"))
			})

			It("returns an error", func() {
				expectUnknownError()
			})
		})
	})

	Describe("PATCH /v3/apps/:guid/environment_variables", func() {
		BeforeEach(func() {
			appRepo.PatchAppEnvVarsReturns(repositories.AppEnvVarsRecord{
				Name:      appGUID + "-env",
				AppGUID:   appGUID,
				SpaceGUID: spaceGUID,
				EnvironmentVariables: map[string]string{
					"KEY0": "VAL0",
					"KEY2": "VAL2",
				},
			}, nil)
			req = createHttpRequest("PATCH", "/v3/apps/"+appGUID+"/environment_variables", strings.NewReader(`{ "var": { "KEY1": null, "KEY2": "VAL2" } }`))
		})

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

		It("fetches the right app", func() {
			Expect(appRepo.GetAppCallCount()).To(Equal(1))
			_, _, actualAppGUID := appRepo.GetAppArgsForCall(0)
			Expect(actualAppGUID).To(Equal(appGUID))
		})

		It("updates the k8s record via the repository", func() {
			Expect(appRepo.PatchAppEnvVarsCallCount()).To(Equal(1))
			_, _, message := appRepo.PatchAppEnvVarsArgsForCall(0)
			Expect(message.AppGUID).To(Equal(appGUID))
			Expect(message.SpaceGUID).To(Equal(spaceGUID))
			Expect(message.EnvironmentVariables).To(Equal(map[string]*string{
				"KEY1": nil,
				"KEY2": tools.PtrTo("VAL2"),
			}))
		})

		DescribeTable("env var validation",
			func(requestBody string, status int) {
				tableTestRecorder := httptest.NewRecorder()
				req = createHttpRequest("PATCH", "/v3/apps/"+appGUID+"/environment_variables", strings.NewReader(requestBody))
				routerBuilder.Build().ServeHTTP(tableTestRecorder, req)
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

		When("the request JSON is invalid", func() {
			BeforeEach(func() {
				req = createHttpRequest("PATCH", "/v3/apps/"+appGUID+"/environment_variables", strings.NewReader(`{`))
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

func createHttpRequest(method string, url string, body io.Reader) *http.Request {
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	Expect(err).NotTo(HaveOccurred())

	return req
}
