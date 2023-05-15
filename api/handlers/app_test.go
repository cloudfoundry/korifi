package handlers_test

import (
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
	. "code.cloudfoundry.org/korifi/tests/matchers"
	"code.cloudfoundry.org/korifi/tools"

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
		appRepo     *fake.CFAppRepository
		dropletRepo *fake.CFDropletRepository
		processRepo *fake.CFProcessRepository
		routeRepo   *fake.CFRouteRepository
		domainRepo  *fake.CFDomainRepository
		spaceRepo   *fake.SpaceRepository
		packageRepo *fake.CFPackageRepository
		req         *http.Request

		appRecord repositories.AppRecord
	)

	BeforeEach(func() {
		appRepo = new(fake.CFAppRepository)
		dropletRepo = new(fake.CFDropletRepository)
		processRepo = new(fake.CFProcessRepository)
		routeRepo = new(fake.CFRouteRepository)
		domainRepo = new(fake.CFDomainRepository)
		spaceRepo = new(fake.SpaceRepository)
		packageRepo = new(fake.CFPackageRepository)
		decoderValidator, err := NewGoPlaygroundValidator()
		Expect(err).NotTo(HaveOccurred())

		apiHandler := NewApp(
			*serverURL,
			appRepo,
			dropletRepo,
			processRepo,
			routeRepo,
			domainRepo,
			spaceRepo,
			packageRepo,
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

		It("returns the App", func() {
			Expect(appRepo.GetAppCallCount()).To(Equal(1))
			_, actualAuthInfo, _ := appRepo.GetAppArgsForCall(0)
			Expect(actualAuthInfo).To(Equal(authInfo))

			Expect(rr).To(HaveHTTPStatus(http.StatusOK))
			Expect(rr).To(HaveHTTPHeaderWithValue("Content-Type", "application/json"))

			Expect(rr).To(HaveHTTPBody(SatisfyAll(
				MatchJSONPath("$.guid", "test-app-guid"),
				MatchJSONPath("$.state", "STOPPED"),
				MatchJSONPath("$.links.self.href", HavePrefix("https://api.example.org")),
			)))
		})

		When("the app is not accessible", func() {
			BeforeEach(func() {
				appRepo.GetAppReturns(repositories.AppRecord{}, apierrors.NewForbiddenError(nil, repositories.AppResourceType))
			})

			It("returns an error", func() {
				expectNotFoundError("App")
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

		It("returns the App", func() {
			Expect(appRepo.CreateAppCallCount()).To(Equal(1))
			_, actualAuthInfo, _ := appRepo.CreateAppArgsForCall(0)
			Expect(actualAuthInfo).To(Equal(authInfo))

			Expect(rr).To(HaveHTTPStatus(http.StatusCreated))
			Expect(rr).To(HaveHTTPHeaderWithValue("Content-Type", "application/json"))

			Expect(rr).To(HaveHTTPBody(SatisfyAll(
				MatchJSONPath("$.guid", appGUID),
				MatchJSONPath("$.state", "STOPPED"),
				MatchJSONPath("$.links.self.href", HavePrefix("https://api.example.org")),
			)))
		})

		It("creates the `web` process", func() {
			Expect(processRepo.CreateProcessCallCount()).To(Equal(1))
			_, actualAuthInfo, actualMsg := processRepo.CreateProcessArgsForCall(0)
			Expect(actualAuthInfo).To(Equal(authInfo))
			Expect(actualMsg).To(Equal(repositories.CreateProcessMessage{
				AppGUID:   appGUID,
				SpaceGUID: spaceGUID,
				Type:      "web",
			}))
		})

		When("creating the process fails", func() {
			BeforeEach(func() {
				processRepo.CreateProcessReturns(errors.New("create-process-err"))
			})

			It("returns an error", func() {
				expectUnknownError()
			})
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

			It("has the expected error", func() {
				expectBadRequestError()
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

			It("returns an unprocessable entity error", func() {
				Expect(rr).To(HaveHTTPStatus(http.StatusUnprocessableEntity))
				Expect(rr).To(HaveHTTPHeaderWithValue("Content-Type", "application/json"))
				Expect(rr).To(HaveHTTPBody(SatisfyAll(
					MatchJSONPath("$.errors", HaveLen(1)),
					MatchJSONPath("$.errors[0].title", "CF-UnprocessableEntity"),
					MatchJSONPath("$.errors[0].code", BeEquivalentTo(10008)),
					MatchJSONPath("$.errors[0].detail", SatisfyAll(
						ContainSubstring("Type is a required field"),
						ContainSubstring("Buildpacks is a required field"),
						ContainSubstring("Stack is a required field"),
					)),
				)))
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

		It("returns the list of apps", func() {
			Expect(appRepo.ListAppsCallCount()).To(Equal(1))
			_, actualAuthInfo, _ := appRepo.ListAppsArgsForCall(0)
			Expect(actualAuthInfo).To(Equal(authInfo))

			Expect(rr).Should(HaveHTTPStatus(http.StatusOK))
			Expect(rr).To(HaveHTTPHeaderWithValue("Content-Type", "application/json"))
			Expect(rr).To(HaveHTTPBody(SatisfyAll(
				MatchJSONPath("$.pagination.first.href", "https://api.example.org/v3/apps"),
				MatchJSONPath("$.resources", HaveLen(2)),
				MatchJSONPath("$.resources[0].guid", "first-test-app-guid"),
				MatchJSONPath("$.resources[0].state", "STOPPED"),
				MatchJSONPath("$.resources[1].guid", "second-test-app-guid"),
			)))
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
				Expect(rr).To(HaveHTTPBody(MatchJSONPath("$.pagination.first.href", "https://api.example.org/v3/apps?names=app1,app2&space_guids=space1,space2")))
			})
		})

		Describe("Order results", func() {
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

			DescribeTable("ordering results", func(orderBy string, expectedOrder ...any) {
				req = createHttpRequest("GET", "/v3/apps?order_by="+orderBy, nil)
				rr = httptest.NewRecorder()
				routerBuilder.Build().ServeHTTP(rr, req)
				Expect(rr).To(HaveHTTPBody(MatchJSONPath("$.resources[*].guid", expectedOrder)))
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

			It("returns an empty response", func() {
				Expect(rr).To(HaveHTTPStatus(http.StatusOK))
				Expect(rr).To(HaveHTTPHeaderWithValue("Content-Type", "application/json"))
				Expect(rr).To(HaveHTTPBody(SatisfyAll(
					MatchJSONPath("$.pagination.total_results", BeEquivalentTo(0)),
					MatchJSONPath("$.resources", BeEmpty()),
				)))
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
			appRecord.GUID = "patched-app-guid"
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
			Expect(rr).To(HaveHTTPStatus(http.StatusOK))
			Expect(rr).To(HaveHTTPHeaderWithValue("Content-Type", "application/json"))
			Expect(rr).To(HaveHTTPBody(SatisfyAll(
				MatchJSONPath("$.guid", "patched-app-guid"),
				MatchJSONPath("$.state", "STOPPED"),
				MatchJSONPath("$.links.self.href", HavePrefix("https://api.example.org")),
			)))
		})

		When("the user doesn't have permission to get the App", func() {
			BeforeEach(func() {
				appRepo.GetAppReturns(repositories.AppRecord{}, apierrors.NewForbiddenError(nil, repositories.AppResourceType))
			})

			It("returns a not found error", func() {
				expectNotFoundError("App")
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

		It("sets the current droplet on the app", func() {
			Expect(appRepo.GetAppCallCount()).To(Equal(1))
			_, _, actualAppGUID := appRepo.GetAppArgsForCall(0)
			Expect(actualAppGUID).To(Equal(appGUID))

			Expect(dropletRepo.GetDropletCallCount()).To(Equal(1))
			_, _, actualDropletGUID := dropletRepo.GetDropletArgsForCall(0)
			Expect(actualDropletGUID).To(Equal(dropletGUID))

			Expect(appRepo.SetCurrentDropletCallCount()).To(Equal(1))
			_, _, message := appRepo.SetCurrentDropletArgsForCall(0)
			Expect(message.AppGUID).To(Equal(appGUID))
			Expect(message.DropletGUID).To(Equal(dropletGUID))
			Expect(message.SpaceGUID).To(Equal(spaceGUID))
		})

		It("returns the droplet", func() {
			Expect(rr).To(HaveHTTPStatus(http.StatusOK))
			Expect(rr).To(HaveHTTPHeaderWithValue("Content-Type", "application/json"))
			Expect(rr).To(HaveHTTPBody(SatisfyAll(
				MatchJSONPath("$.data.guid", dropletGUID),
				MatchJSONPath("$.links.self.href", HavePrefix("https://api.example.org")),
			)))
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
				expectNotFoundError("App")
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

		It("returns the App in the response with a state of STARTED", func() {
			Expect(rr).To(HaveHTTPStatus(http.StatusOK))
			Expect(rr).To(HaveHTTPHeaderWithValue("Content-Type", "application/json"))
			Expect(rr).To(HaveHTTPBody(SatisfyAll(
				MatchJSONPath("$.guid", appGUID),
				MatchJSONPath("$.state", "STARTED"),
				MatchJSONPath("$.links.self.href", HavePrefix("https://api.example.org")),
			)))
		})

		When("getting the app is forbidden", func() {
			BeforeEach(func() {
				appRepo.GetAppReturns(repositories.AppRecord{}, apierrors.NewForbiddenError(nil, repositories.AppResourceType))
			})

			It("returns an error", func() {
				expectNotFoundError("App")
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

		It("returns the App in the response with a state of STOPPED", func() {
			Expect(rr).To(HaveHTTPHeaderWithValue("Content-Type", "application/json"))
			Expect(rr).To(HaveHTTPStatus(http.StatusOK))
			Expect(rr).To(HaveHTTPBody(SatisfyAll(
				MatchJSONPath("$.guid", appGUID),
				MatchJSONPath("$.state", "STOPPED"),
				MatchJSONPath("$.links.self.href", HavePrefix("https://api.example.org")),
			)))
		})

		When("fetching the app is forbidden", func() {
			BeforeEach(func() {
				appRepo.GetAppReturns(repositories.AppRecord{}, apierrors.NewForbiddenError(nil, "App"))
			})

			It("returns a not found error", func() {
				expectNotFoundError("App")
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

		It("returns the processes", func() {
			Expect(rr).To(HaveHTTPStatus(http.StatusOK))
			Expect(rr).To(HaveHTTPHeaderWithValue("Content-Type", "application/json"))
			Expect(rr).To(HaveHTTPBody(SatisfyAll(
				MatchJSONPath("$.pagination.first.href", "https://api.example.org/v3/apps/"+appGUID+"/processes"),
				MatchJSONPath("$.resources", HaveLen(2)),
				MatchJSONPath("$.resources[0].guid", "process-1-guid"),
				MatchJSONPath("$.resources[0].command", "[PRIVATE DATA HIDDEN IN LISTS]"),
				MatchJSONPath("$.resources[1].guid", "process-2-guid"),
				MatchJSONPath("$.resources[1].command", "[PRIVATE DATA HIDDEN IN LISTS]"),
			)))
		})

		When("the app cannot be accessed", func() {
			BeforeEach(func() {
				appRepo.GetAppReturns(repositories.AppRecord{}, apierrors.NewForbiddenError(nil, repositories.AppResourceType))
			})

			It("returns an error", func() {
				expectNotFoundError("App")
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

		It("returns a process", func() {
			Expect(processRepo.GetProcessByAppTypeAndSpaceCallCount()).To(Equal(1))
			_, actualAuthInfo, _, _, _ := processRepo.GetProcessByAppTypeAndSpaceArgsForCall(0)
			Expect(actualAuthInfo).To(Equal(authInfo))

			Expect(rr).To(HaveHTTPStatus(http.StatusOK))
			Expect(rr).To(HaveHTTPHeaderWithValue("Content-Type", "application/json"))
			Expect(rr).To(HaveHTTPBody(SatisfyAll(
				MatchJSONPath("$.guid", "process-1-guid"),
				MatchJSONPath("$.command", "bundle exec rackup config.ru -p $PORT -o 0.0.0.0"),
				MatchJSONPath("$.links.self.href", HavePrefix("https://api.example.org")),
			)))
		})

		When("the user lacks access in the app namespace", func() {
			BeforeEach(func() {
				appRepo.GetAppReturns(repositories.AppRecord{}, apierrors.NewForbiddenError(errors.New("Forbidden"), repositories.AppResourceType))
			})

			It("returns an not found error", func() {
				expectNotFoundError("App")
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
			processRepo.ListProcessesReturns([]repositories.ProcessRecord{
				{
					GUID:      "process-1-guid",
					SpaceGUID: spaceGUID,
					AppGUID:   appGUID,
					Type:      "web",
				},
			}, nil)

			processRepo.ScaleProcessReturns(repositories.ProcessRecord{
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

		It("gets the app", func() {
			Expect(appRepo.GetAppCallCount()).To(Equal(1))
			_, actualAuthInfo, actualAppGUID := appRepo.GetAppArgsForCall(0)
			Expect(actualAuthInfo).To(Equal(authInfo))
			Expect(actualAppGUID).To(Equal(appGUID))
		})

		It("lists the app processes", func() {
			Expect(processRepo.ListProcessesCallCount()).To(Equal(1))
			_, actualAuthInfo, listMsg := processRepo.ListProcessesArgsForCall(0)
			Expect(actualAuthInfo).To(Equal(authInfo))
			Expect(listMsg).To(Equal(repositories.ListProcessesMessage{
				AppGUIDs:  []string{appGUID},
				SpaceGUID: spaceGUID,
			}))
		})

		It("scales the process", func() {
			Expect(processRepo.ScaleProcessCallCount()).To(Equal(1), "did not call scaleProcess just once")
			_, actualAuthInfo, scaleMsg := processRepo.ScaleProcessArgsForCall(0)
			Expect(actualAuthInfo).To(Equal(authInfo))
			Expect(scaleMsg).To(Equal(repositories.ScaleProcessMessage{
				GUID:      "process-1-guid",
				SpaceGUID: spaceGUID,
				ProcessScaleValues: repositories.ProcessScaleValues{
					Instances: tools.PtrTo(5),
					MemoryMB:  tools.PtrTo[int64](256),
					DiskMB:    tools.PtrTo[int64](1024),
				},
			}))
		})

		It("returns the scaled process", func() {
			Expect(rr).To(HaveHTTPStatus(http.StatusOK))
			Expect(rr).To(HaveHTTPHeaderWithValue("Content-Type", "application/json"))
			Expect(rr).To(HaveHTTPBody(SatisfyAll(
				MatchJSONPath("$.guid", "process-1-guid"),
				MatchJSONPath("$.command", "bundle exec rackup config.ru -p $PORT -o 0.0.0.0"),
				MatchJSONPath("$.instances", BeEquivalentTo(5)),
				MatchJSONPath("$.links.self.href", HavePrefix("https://api.example.org")),
			)))
		})

		When("the request JSON is invalid", func() {
			BeforeEach(func() {
				req = createHttpRequest("POST", "/v3/apps/"+appGUID+"/processes/web/actions/scale", strings.NewReader(`}`))
			})

			It("returns an error", func() {
				expectBadRequestError()
			})
		})

		When("the user does not have permissions to get the app", func() {
			BeforeEach(func() {
				appRepo.GetAppReturns(repositories.AppRecord{}, apierrors.NewForbiddenError(nil, "App"))
			})

			It("return a NotFound error", func() {
				expectNotFoundError("App")
			})
		})

		When("getting the app fails", func() {
			BeforeEach(func() {
				appRepo.GetAppReturns(repositories.AppRecord{}, errors.New("boom"))
			})

			It("return an unknown error", func() {
				expectUnknownError()
			})
		})

		When("listing the app processes fails", func() {
			BeforeEach(func() {
				processRepo.ListProcessesReturns([]repositories.ProcessRecord{}, errors.New("boom"))
			})

			It("return an unknown error", func() {
				expectUnknownError()
			})
		})

		When("scaling a nonexistent process type", func() {
			BeforeEach(func() {
				req = createHttpRequest("POST", "/v3/apps/"+appGUID+"/processes/bob/actions/scale", strings.NewReader("{}"))
			})

			It("return a NotFound error", func() {
				expectNotFoundError("Process")
			})
		})

		When("there is an error scaling the app", func() {
			BeforeEach(func() {
				processRepo.ScaleProcessReturns(repositories.ProcessRecord{}, errors.New("unknown!"))
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
				Expect(tableTestRecorder).To(HaveHTTPStatus(status))
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

		It("returns the list of routes", func() {
			Expect(rr).To(HaveHTTPStatus(http.StatusOK))
			Expect(rr).To(HaveHTTPHeaderWithValue("Content-Type", "application/json"))
			Expect(rr).To(HaveHTTPBody(SatisfyAll(
				MatchJSONPath("$.pagination.total_results", BeEquivalentTo(1)),
				MatchJSONPath("$.pagination.first.href", "https://api.example.org/v3/apps/"+appGUID+"/routes"),
				MatchJSONPath("$.resources", HaveLen(1)),
				MatchJSONPath("$.resources[0].guid", "test-route-guid"),
				MatchJSONPath("$.resources[0].url", "test-route-host.example.org/some_path"),
			)))
		})

		When("the app cannot be accessed", func() {
			BeforeEach(func() {
				appRepo.GetAppReturns(repositories.AppRecord{}, apierrors.NewForbiddenError(nil, repositories.AppResourceType))
			})

			It("returns an error", func() {
				expectNotFoundError("App")
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

		It("returns the current droplet", func() {
			Expect(appRepo.GetAppCallCount()).To(Equal(1))
			_, actualAuthInfo, actualAppGUID := appRepo.GetAppArgsForCall(0)
			Expect(actualAuthInfo).To(Equal(authInfo))
			Expect(actualAppGUID).To(Equal(appGUID))

			Expect(dropletRepo.GetDropletCallCount()).To(Equal(1))
			_, actualAuthInfo, actualDropletGUID := dropletRepo.GetDropletArgsForCall(0)
			Expect(actualAuthInfo).To(Equal(authInfo))
			Expect(actualDropletGUID).To(Equal(dropletGUID))

			Expect(rr).To(HaveHTTPStatus(http.StatusOK))
			Expect(rr).To(HaveHTTPHeaderWithValue("Content-Type", "application/json"))
			Expect(rr).To(HaveHTTPBody(SatisfyAll(
				MatchJSONPath("$.guid", dropletGUID),
				MatchJSONPath("$.state", "STAGED"),
				MatchJSONPath("$.links.self.href", HavePrefix("https://api.example.org")),
			)))
		})

		When("the app is not accessible", func() {
			BeforeEach(func() {
				appRepo.GetAppReturns(repositories.AppRecord{}, apierrors.NewForbiddenError(nil, repositories.AppResourceType))
			})

			It("returns an error", func() {
				expectNotFoundError("App")
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
				expectErrorResponse(http.StatusNotFound, "CF-ResourceNotFound", "Droplet not found", 10010)
			})
		})

		When("the user cannot access the droplet", func() {
			BeforeEach(func() {
				dropletRepo.GetDropletReturns(repositories.DropletRecord{}, apierrors.NewForbiddenError(nil, repositories.DropletResourceType))
			})

			It("returns a NotFound error with code 10010 (that is ignored by the cf cli)", func() {
				expectErrorResponse(http.StatusNotFound, "CF-ResourceNotFound", "Droplet not found", 10010)
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

		It("restarts the app", func() {
			Expect(appRepo.GetAppCallCount()).To(Equal(1))
			_, actualAuthInfo, actualAppGUID := appRepo.GetAppArgsForCall(0)
			Expect(actualAuthInfo).To(Equal(authInfo))
			Expect(actualAppGUID).To(Equal(appGUID))

			Expect(appRepo.SetAppDesiredStateCallCount()).To(Equal(2))
			_, actualAuthInfo, appDesiredStateMessage := appRepo.SetAppDesiredStateArgsForCall(0)
			Expect(actualAuthInfo).To(Equal(authInfo))
			Expect(appDesiredStateMessage.DesiredState).To(Equal("STOPPED"))
			_, actualAuthInfo, appDesiredStateMessage = appRepo.SetAppDesiredStateArgsForCall(1)
			Expect(actualAuthInfo).To(Equal(authInfo))
			Expect(appDesiredStateMessage.DesiredState).To(Equal("STARTED"))

			Expect(rr).To(HaveHTTPStatus(http.StatusOK))
			Expect(rr).To(HaveHTTPHeaderWithValue("Content-Type", "application/json"))
			Expect(rr).To(HaveHTTPBody(SatisfyAll(
				MatchJSONPath("$.guid", "test-app-guid"),
				MatchJSONPath("$.state", "STARTED"),
				MatchJSONPath("$.links.self.href", HavePrefix("https://api.example.org")),
			)))
		})

		When("no permissions to get the app", func() {
			BeforeEach(func() {
				appRepo.GetAppReturns(repositories.AppRecord{}, apierrors.NewForbiddenError(nil, repositories.AppResourceType))
			})

			It("returns an error", func() {
				expectNotFoundError("App")
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
				Expect(rr).To(HaveHTTPStatus(http.StatusOK))
			})

			It("returns the app in the response with a state of STARTED", func() {
				Expect(rr).To(HaveHTTPBody(ContainSubstring(`"state":"STARTED"`)))
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

		It("deletes the app", func() {
			Expect(appRepo.GetAppCallCount()).To(Equal(1))
			_, _, actualAppGUID := appRepo.GetAppArgsForCall(0)
			Expect(actualAppGUID).To(Equal(appGUID))

			Expect(appRepo.DeleteAppCallCount()).To(Equal(1))
			_, _, message := appRepo.DeleteAppArgsForCall(0)
			Expect(message.AppGUID).To(Equal(appGUID))
			Expect(message.SpaceGUID).To(Equal(spaceGUID))

			Expect(rr).To(HaveHTTPStatus(http.StatusAccepted))
			Expect(rr).To(HaveHTTPHeaderWithValue("Location", "https://api.example.org/v3/jobs/app.delete~"+appGUID))
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
				EnvironmentVariables: map[string]string{"VAR": "VAL"},
			}, nil)

			req = createHttpRequest("GET", "/v3/apps/"+appGUID+"/env", nil)
		})

		It("returns the app environment", func() {
			Expect(appRepo.GetAppEnvCallCount()).To(Equal(1))
			_, actualAuthInfo, _ := appRepo.GetAppEnvArgsForCall(0)
			Expect(actualAuthInfo).To(Equal(authInfo))

			Expect(rr).To(HaveHTTPStatus(http.StatusOK))
			Expect(rr).To(HaveHTTPHeaderWithValue("Content-Type", "application/json"))
			Expect(rr).To(HaveHTTPBody(MatchJSONPath("$.environment_variables.VAR", "VAL")))
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

		It("updates the app environemnt", func() {
			Expect(appRepo.GetAppCallCount()).To(Equal(1))
			_, _, actualAppGUID := appRepo.GetAppArgsForCall(0)
			Expect(actualAppGUID).To(Equal(appGUID))

			Expect(appRepo.PatchAppEnvVarsCallCount()).To(Equal(1))
			_, _, message := appRepo.PatchAppEnvVarsArgsForCall(0)
			Expect(message.AppGUID).To(Equal(appGUID))
			Expect(message.SpaceGUID).To(Equal(spaceGUID))
			Expect(message.EnvironmentVariables).To(Equal(map[string]*string{
				"KEY1": nil,
				"KEY2": tools.PtrTo("VAL2"),
			}))

			Expect(rr).To(HaveHTTPStatus(http.StatusOK))
			Expect(rr).To(HaveHTTPHeaderWithValue("Content-Type", "application/json"))
			Expect(rr).To(HaveHTTPBody(SatisfyAll(
				MatchJSONPath("$.var.KEY0", "VAL0"),
				MatchJSONPath("$.var.KEY2", "VAL2"),
				MatchJSONPath("$.links.self.href", HavePrefix("https://api.example.org")),
			)))
		})

		DescribeTable("env var validation",
			func(requestBody string, status int) {
				tableTestRecorder := httptest.NewRecorder()
				req = createHttpRequest("PATCH", "/v3/apps/"+appGUID+"/environment_variables", strings.NewReader(requestBody))
				routerBuilder.Build().ServeHTTP(tableTestRecorder, req)
				Expect(tableTestRecorder).To(HaveHTTPStatus(status))
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

			It("returns a message parse error", func() {
				expectBadRequestError()
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

	Describe("GET /v3/apps/:guid/packages", func() {
		var (
			package1Record repositories.PackageRecord
			package2Record repositories.PackageRecord
		)

		BeforeEach(func() {
			packageRecord := repositories.PackageRecord{
				GUID:        "package-1-guid",
				UID:         "package-1-uid",
				Type:        "bits",
				AppGUID:     appGUID,
				SpaceGUID:   spaceGUID,
				State:       "AWAITING_UPLOAD",
				CreatedAt:   "2017-04-21T18:48:22Z",
				UpdatedAt:   "2017-04-21T18:48:42Z",
				Labels:      map[string]string{},
				Annotations: map[string]string{},
				ImageRef:    "registry.repo/foo",
			}

			package1Record = packageRecord

			package2Record = packageRecord
			package2Record.GUID = "package-2-guid"
			package2Record.UID = "package-2-guid"
			package2Record.State = "READY"

			packageRepo.ListPackagesReturns([]repositories.PackageRecord{
				package1Record,
				package2Record,
			}, nil)

			req = createHttpRequest("GET", "/v3/apps/"+appGUID+"/packages", nil)
		})

		It("returns the packages", func() {
			Expect(rr).To(HaveHTTPStatus(http.StatusOK))
			Expect(rr).To(HaveHTTPHeaderWithValue("Content-Type", "application/json"))
			Expect(rr).To(HaveHTTPBody(SatisfyAll(
				MatchJSONPath("$.pagination.first.href", "https://api.example.org/v3/apps/test-app-guid/packages"),
				MatchJSONPath("$.resources", HaveLen(2)),
				MatchJSONPath("$.resources[0].guid", "package-1-guid"),
				MatchJSONPath("$.resources[0].state", "AWAITING_UPLOAD"),
				MatchJSONPath("$.resources[1].guid", "package-2-guid"),
			)))
		})

		When("the app cannot be accessed", func() {
			BeforeEach(func() {
				appRepo.GetAppReturns(repositories.AppRecord{}, apierrors.NewForbiddenError(nil, repositories.AppResourceType))
			})

			It("returns an error", func() {
				expectNotFoundError("App")
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

		When("there is some error fetching the app's packages", func() {
			BeforeEach(func() {
				packageRepo.ListPackagesReturns([]repositories.PackageRecord{}, errors.New("unknown!"))
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
