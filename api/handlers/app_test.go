package handlers_test

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	apierrors "code.cloudfoundry.org/korifi/api/errors"
	. "code.cloudfoundry.org/korifi/api/handlers"
	"code.cloudfoundry.org/korifi/api/handlers/fake"
	"code.cloudfoundry.org/korifi/api/handlers/stats"
	"code.cloudfoundry.org/korifi/api/payloads"
	"code.cloudfoundry.org/korifi/api/repositories"
	"code.cloudfoundry.org/korifi/api/repositories/k8sklient/descriptors"
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
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
		appRepo                 *fake.CFAppRepository
		dropletRepo             *fake.CFDropletRepository
		processRepo             *fake.CFProcessRepository
		routeRepo               *fake.CFRouteRepository
		domainRepo              *fake.CFDomainRepository
		spaceRepo               *fake.CFSpaceRepository
		packageRepo             *fake.CFPackageRepository
		podRepo                 *fake.PodRepository
		requestValidator        *fake.RequestValidator
		gaugesCollector         *fake.GaugesCollector
		instancesStateCollector *fake.InstancesStateCollector
		req                     *http.Request

		appRecord repositories.AppRecord
	)

	BeforeEach(func() {
		appRepo = new(fake.CFAppRepository)
		dropletRepo = new(fake.CFDropletRepository)
		processRepo = new(fake.CFProcessRepository)
		routeRepo = new(fake.CFRouteRepository)
		domainRepo = new(fake.CFDomainRepository)
		spaceRepo = new(fake.CFSpaceRepository)
		packageRepo = new(fake.CFPackageRepository)
		requestValidator = new(fake.RequestValidator)
		podRepo = new(fake.PodRepository)
		gaugesCollector = new(fake.GaugesCollector)
		instancesStateCollector = new(fake.InstancesStateCollector)

		apiHandler := NewApp(
			*serverURL,
			appRepo,
			dropletRepo,
			processRepo,
			routeRepo,
			domainRepo,
			spaceRepo,
			packageRepo,
			requestValidator,
			podRepo,
			gaugesCollector,
			instancesStateCollector,
		)

		appRecord = repositories.AppRecord{
			GUID:        appGUID,
			Name:        "test-app",
			SpaceGUID:   spaceGUID,
			State:       "STOPPED",
			Revision:    "0",
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
		var payload *payloads.AppCreate

		BeforeEach(func() {
			payload = &payloads.AppCreate{
				Name: appName,
				Relationships: &payloads.AppRelationships{
					Space: &payloads.Relationship{
						Data: &payloads.RelationshipData{
							GUID: spaceGUID,
						},
					},
				},
			}
			requestValidator.DecodeAndValidateJSONPayloadStub = decodeAndValidatePayloadStub(payload)
			appRepo.CreateAppReturns(appRecord, nil)
			req = createHttpRequest("POST", "/v3/apps", strings.NewReader("the-json-body"))
		})

		It("returns the App", func() {
			Expect(rr).To(HaveHTTPStatus(http.StatusCreated))
			Expect(rr).To(HaveHTTPHeaderWithValue("Content-Type", "application/json"))

			Expect(rr).To(HaveHTTPBody(SatisfyAll(
				MatchJSONPath("$.guid", appGUID),
				MatchJSONPath("$.state", "STOPPED"),
				MatchJSONPath("$.links.self.href", HavePrefix("https://api.example.org")),
			)))
		})

		It("sends an AppCreate message with default lifecycle to the repository", func() {
			Expect(appRepo.CreateAppCallCount()).To(Equal(1))
			_, actualAuthInfo, actualCreateMessage := appRepo.CreateAppArgsForCall(0)
			Expect(actualAuthInfo).To(Equal(authInfo))
			Expect(actualCreateMessage).To(Equal(repositories.CreateAppMessage{
				Name:      appName,
				SpaceGUID: spaceGUID,
				State:     "STOPPED",
				Lifecycle: repositories.Lifecycle{
					Type: "buildpack",
					Data: repositories.LifecycleData{
						Stack: "cflinuxfs3",
					},
				},
			}))
		})

		When("the app has buildpack lifecycle", func() {
			BeforeEach(func() {
				payload.Lifecycle = &payloads.Lifecycle{
					Type: "buildpack",
					Data: &payloads.LifecycleData{
						Buildpacks: []string{"bp1"},
						Stack:      "my-stack",
					},
				}
			})

			It("sends an AppCreate message with buildpack lifecycle", func() {
				Expect(appRepo.CreateAppCallCount()).To(Equal(1))
				_, _, actualCreateMessage := appRepo.CreateAppArgsForCall(0)
				Expect(actualCreateMessage.Lifecycle).To(Equal(
					repositories.Lifecycle{
						Type: "buildpack",
						Data: repositories.LifecycleData{
							Stack:      "my-stack",
							Buildpacks: []string{"bp1"},
						},
					}))
			})
		})

		When("the app has docker lifecycle", func() {
			BeforeEach(func() {
				payload.Lifecycle = &payloads.Lifecycle{
					Type: "docker",
					Data: &payloads.LifecycleData{},
				}
			})

			It("sends an AppCreate message with docker lifecycle and empty data", func() {
				Expect(appRepo.CreateAppCallCount()).To(Equal(1))
				_, _, actualCreateMessage := appRepo.CreateAppArgsForCall(0)
				Expect(actualCreateMessage.Lifecycle).To(Equal(
					repositories.Lifecycle{
						Type: "docker",
						Data: repositories.LifecycleData{},
					}))
			})
		})

		It("validates the payload", func() {
			Expect(requestValidator.DecodeAndValidateJSONPayloadCallCount()).To(Equal(1))
			actualReq, _ := requestValidator.DecodeAndValidateJSONPayloadArgsForCall(0)
			Expect(bodyString(actualReq)).To(Equal("the-json-body"))
		})

		When("creating the app fails", func() {
			BeforeEach(func() {
				appRepo.CreateAppReturns(repositories.AppRecord{}, errors.New("create-app-err"))
			})

			It("returns an error", func() {
				expectUnknownError()
			})
		})

		When("the request body is invalid", func() {
			BeforeEach(func() {
				requestValidator.DecodeAndValidateJSONPayloadReturns(apierrors.NewUnprocessableEntityError(errors.New("validation-err"), "validation error"))
			})

			It("returns an unprocessable entity error", func() {
				Expect(rr).To(HaveHTTPStatus(http.StatusUnprocessableEntity))
				Expect(rr).To(HaveHTTPHeaderWithValue("Content-Type", "application/json"))
				Expect(rr).To(HaveHTTPBody(SatisfyAll(
					MatchJSONPath("$.errors", HaveLen(1)),
					MatchJSONPath("$.errors[0].title", "CF-UnprocessableEntity"),
					MatchJSONPath("$.errors[0].code", BeEquivalentTo(10008)),
					MatchJSONPath("$.errors[0].detail", Equal("validation error")),
				)))
			})
		})

		When("the space does not exist", func() {
			BeforeEach(func() {
				spaceRepo.GetSpaceReturns(repositories.SpaceRecord{}, apierrors.NewNotFoundError(nil, repositories.SpaceResourceType))
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
			appRepo.ListAppsReturns(repositories.ListResult[repositories.AppRecord]{Records: []repositories.AppRecord{
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
			}}, nil)

			req = createHttpRequest("GET", "/v3/apps?foo=bar", nil)
		})

		It("returns the list of apps", func() {
			Expect(requestValidator.DecodeAndValidateURLValuesCallCount()).To(Equal(1))
			actualReq, _ := requestValidator.DecodeAndValidateURLValuesArgsForCall(0)
			Expect(actualReq.URL.String()).To(HaveSuffix("foo=bar"))

			Expect(appRepo.ListAppsCallCount()).To(Equal(1))
			_, actualAuthInfo, _ := appRepo.ListAppsArgsForCall(0)
			Expect(actualAuthInfo).To(Equal(authInfo))

			Expect(rr).Should(HaveHTTPStatus(http.StatusOK))
			Expect(rr).To(HaveHTTPHeaderWithValue("Content-Type", "application/json"))
			Expect(rr).To(HaveHTTPBody(SatisfyAll(
				MatchJSONPath("$.resources", HaveLen(2)),
				MatchJSONPath("$.resources[0].guid", "first-test-app-guid"),
				MatchJSONPath("$.resources[0].state", "STOPPED"),
				MatchJSONPath("$.resources[1].guid", "second-test-app-guid"),
			)))
		})

		When("filtering query params are provided", func() {
			BeforeEach(func() {
				requestValidator.DecodeAndValidateURLValuesStub = decodeAndValidateURLValuesStub(&payloads.AppList{
					Names:      "a1,a2",
					GUIDs:      "g1,g2",
					SpaceGUIDs: "s1,s2",
				})
			})

			It("passes them to the repository", func() {
				Expect(appRepo.ListAppsCallCount()).To(Equal(1))
				_, _, message := appRepo.ListAppsArgsForCall(0)

				Expect(message.Names).To(ConsistOf("a1", "a2"))
				Expect(message.SpaceGUIDs).To(ConsistOf("s1", "s2"))
				Expect(message.Guids).To(ConsistOf("g1", "g2"))
			})
		})

		When("no apps can be found", func() {
			BeforeEach(func() {
				appRepo.ListAppsReturns(repositories.ListResult[repositories.AppRecord]{}, nil)
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
				appRepo.ListAppsReturns(repositories.ListResult[repositories.AppRecord]{}, errors.New("unknown!"))
			})

			It("returns an error", func() {
				expectUnknownError()
			})
		})

		When("invalid query parameters are provided", func() {
			BeforeEach(func() {
				requestValidator.DecodeAndValidateURLValuesReturns(errors.New("boom"))
			})

			It("returns an error", func() {
				expectUnknownError()
			})
		})
	})

	Describe("PATCH /v3/apps/:guid", func() {
		var payload *payloads.AppPatch

		BeforeEach(func() {
			appRecord.GUID = "patched-app-guid"
			payload = &payloads.AppPatch{
				Metadata: payloads.MetadataPatch{
					Annotations: map[string]*string{
						"hello":                       tools.PtrTo("there"),
						"foo.example.com/lorem-ipsum": tools.PtrTo("Lorem ipsum."),
					},
					Labels: map[string]*string{
						"env":                           tools.PtrTo("production"),
						"foo.example.com/my-identifier": tools.PtrTo("aruba"),
					},
				},
				Lifecycle: &payloads.LifecyclePatch{
					Type: "buildpack",
					Data: &payloads.LifecycleDataPatch{
						Buildpacks: &[]string{"my-buildpack"},
						Stack:      "cflinuxfs3",
					},
				},
			}
			requestValidator.DecodeAndValidateJSONPayloadStub = decodeAndValidatePayloadStub(payload)

			appRepo.PatchAppReturns(appRecord, nil)
			req = createHttpRequest("PATCH", "/v3/apps/"+appGUID, strings.NewReader("the-json-body"))
		})

		It("patches the app", func() {
			Expect(appRepo.PatchAppCallCount()).To(Equal(1))
			_, _, msg := appRepo.PatchAppArgsForCall(0)
			Expect(msg.AppGUID).To(Equal(appGUID))
			Expect(msg.SpaceGUID).To(Equal(spaceGUID))
			Expect(msg.Annotations).To(HaveKeyWithValue("hello", PointTo(Equal("there"))))
			Expect(msg.Annotations).To(HaveKeyWithValue("foo.example.com/lorem-ipsum", PointTo(Equal("Lorem ipsum."))))
			Expect(msg.Labels).To(HaveKeyWithValue("env", PointTo(Equal("production"))))
			Expect(msg.Labels).To(HaveKeyWithValue("foo.example.com/my-identifier", PointTo(Equal("aruba"))))
			Expect(*msg.Lifecycle).To(Equal(repositories.LifecyclePatch{
				Type: tools.PtrTo("buildpack"),
				Data: &repositories.LifecycleDataPatch{
					Buildpacks: &[]string{"my-buildpack"},
					Stack:      "cflinuxfs3",
				},
			}))
		})

		It("validates the payload", func() {
			Expect(requestValidator.DecodeAndValidateJSONPayloadCallCount()).To(Equal(1))
			actualReq, _ := requestValidator.DecodeAndValidateJSONPayloadArgsForCall(0)
			Expect(bodyString(actualReq)).To(Equal("the-json-body"))
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
				Expect(appRepo.PatchAppCallCount()).To(Equal(0))
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
				Expect(appRepo.PatchAppCallCount()).To(Equal(0))
			})
		})

		When("patching the App errors", func() {
			BeforeEach(func() {
				appRepo.PatchAppReturns(repositories.AppRecord{}, errors.New("boom"))
			})

			It("returns an error", func() {
				expectUnknownError()
			})
		})

		When("the request body is invalid", func() {
			BeforeEach(func() {
				requestValidator.DecodeAndValidateJSONPayloadReturns(apierrors.NewUnprocessableEntityError(errors.New("validation-err"), "validation error"))
			})

			It("returns an unprocessable entity error", func() {
				expectUnprocessableEntityError("validation error")
			})
		})
	})

	Describe("PATCH /v3/apps/:guid/relationships/current_droplet", func() {
		var (
			droplet repositories.DropletRecord
			payload *payloads.AppSetCurrentDroplet
		)

		BeforeEach(func() {
			droplet = repositories.DropletRecord{GUID: dropletGUID, AppGUID: appGUID}

			dropletRepo.GetDropletReturns(droplet, nil)
			appRepo.SetCurrentDropletReturns(repositories.CurrentDropletRecord{
				AppGUID:     appGUID,
				DropletGUID: dropletGUID,
			}, nil)

			payload = &payloads.AppSetCurrentDroplet{
				Relationship: payloads.Relationship{
					Data: &payloads.RelationshipData{
						GUID: dropletGUID,
					},
				},
			}
			requestValidator.DecodeAndValidateJSONPayloadStub = decodeAndValidatePayloadStub(payload)

			req = createHttpRequest("PATCH", "/v3/apps/"+appGUID+"/relationships/current_droplet", strings.NewReader("the-json-body"))
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

		It("validates the payload", func() {
			Expect(requestValidator.DecodeAndValidateJSONPayloadCallCount()).To(Equal(1))
			actualReq, _ := requestValidator.DecodeAndValidateJSONPayloadArgsForCall(0)
			Expect(bodyString(actualReq)).To(Equal("the-json-body"))
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
		When("the request body is invalid", func() {
			BeforeEach(func() {
				requestValidator.DecodeAndValidateJSONPayloadReturns(apierrors.NewUnprocessableEntityError(errors.New("validation-err"), "validation error"))
			})

			It("returns an unprocessable entity error", func() {
				expectUnprocessableEntityError("validation error")
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
			appRepo.PatchAppReturns(updatedAppRecord, nil)

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
				GUID:    "process-1-guid",
				Command: "rackup",
			}

			process1Record = processRecord

			process2Record = processRecord
			process2Record.GUID = "process-2-guid"
			process2Record.Type = "worker"

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
			processRepo.ListProcessesReturns([]repositories.ProcessRecord{{
				GUID:    "process-1-guid",
				Command: "bundle exec rackup config.ru -p $PORT -o 0.0.0.0",
			}}, nil)

			req = createHttpRequest("GET", "/v3/apps/"+appGUID+"/processes/web", nil)
		})

		It("returns a process", func() {
			Expect(processRepo.ListProcessesCallCount()).To(Equal(1))
			_, actualAuthInfo, _ := processRepo.ListProcessesArgsForCall(0)
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
				processRepo.ListProcessesReturns([]repositories.ProcessRecord{}, errors.New("some-error"))
			})

			It("return a process unknown error", func() {
				expectUnknownError()
			})
		})
	})

	Describe("GET /v3/apps/:guid/processes/{type}/stats", func() {
		BeforeEach(func() {
			processRepo.ListProcessesReturns([]repositories.ProcessRecord{{
				GUID: "process-guid",
			}}, nil)

			gaugesCollector.CollectProcessGaugesReturns([]stats.ProcessGauges{{
				Index:    0,
				MemQuota: tools.PtrTo(int64(1024)),
			}, {
				Index:    1,
				MemQuota: tools.PtrTo(int64(512)),
			}}, nil)

			instancesStateCollector.CollectProcessInstancesStatesReturns([]stats.ProcessInstanceState{
				{
					ID:    0,
					Type:  "web",
					State: korifiv1alpha1.InstanceStateRunning,
				},
				{
					ID:    1,
					Type:  "web",
					State: korifiv1alpha1.InstanceStateDown,
				},
			}, nil)

			req = createHttpRequest("GET", "/v3/apps/"+appGUID+"/processes/web/stats", nil)
		})

		It("returns the process stats", func() {
			Expect(gaugesCollector.CollectProcessGaugesCallCount()).To(Equal(1))
			_, actualAppGUID, actualProcessGUID := gaugesCollector.CollectProcessGaugesArgsForCall(0)
			Expect(actualAppGUID).To(Equal(appGUID))
			Expect(actualProcessGUID).To(Equal("process-guid"))

			Expect(instancesStateCollector.CollectProcessInstancesStatesCallCount()).To(Equal(1))
			_, actualProcessGUID = instancesStateCollector.CollectProcessInstancesStatesArgsForCall(0)
			Expect(actualProcessGUID).To(Equal("process-guid"))

			Expect(rr).To(HaveHTTPStatus(http.StatusOK))
			Expect(rr).To(HaveHTTPHeaderWithValue("Content-Type", "application/json"))

			Expect(rr).To(HaveHTTPBody(SatisfyAll(
				MatchJSONPath("$.resources", HaveLen(2)),
				MatchJSONPath("$.resources[0].type", "web"),
				MatchJSONPath("$.resources[0].index", BeEquivalentTo(0)),
				MatchJSONPath("$.resources[0].state", Equal("RUNNING")),
				MatchJSONPath("$.resources[1].type", "web"),
				MatchJSONPath("$.resources[1].mem_quota", BeEquivalentTo(512)),
				MatchJSONPath("$.resources[1].state", Equal("DOWN")),
			)))
		})

		When("getting the app fails", func() {
			BeforeEach(func() {
				appRepo.GetAppReturns(repositories.AppRecord{}, errors.New("get-app"))
			})

			It("returns an error", func() {
				expectUnknownError()
			})
		})

		When("there is an error fetching the process", func() {
			BeforeEach(func() {
				processRepo.ListProcessesReturns([]repositories.ProcessRecord{}, errors.New("some-error"))
			})

			It("return a process unknown error", func() {
				expectUnknownError()
			})
		})

		When("collecting instance state fails", func() {
			BeforeEach(func() {
				instancesStateCollector.CollectProcessInstancesStatesReturns(nil, errors.New("boom"))
			})

			It("returns an error", func() {
				expectUnknownError()
			})
		})

		When("collecting gauges fails", func() {
			BeforeEach(func() {
				gaugesCollector.CollectProcessGaugesReturns(nil, errors.New("boom"))
			})

			It("returns an error", func() {
				expectUnknownError()
			})
		})
	})

	Describe("the POST /v3/apps/:guid/process/:processType/actions/scale endpoint", func() {
		var payload *payloads.ProcessScale

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
				Command:          "bundle exec rackup config.ru -p $PORT -o 0.0.0.0",
				DesiredInstances: 5,
			}, nil)

			payload = &payloads.ProcessScale{
				Instances: tools.PtrTo[int32](5),
				MemoryMB:  tools.PtrTo[int64](256),
				DiskMB:    tools.PtrTo[int64](1024),
			}
			requestValidator.DecodeAndValidateJSONPayloadStub = decodeAndValidatePayloadStub(payload)

			req = createHttpRequest("POST", "/v3/apps/"+appGUID+"/processes/web/actions/scale", strings.NewReader("the-json-body"))
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
				AppGUIDs:   []string{appGUID},
				SpaceGUIDs: []string{spaceGUID},
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
					Instances: tools.PtrTo[int32](5),
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

		It("validates the payload", func() {
			Expect(requestValidator.DecodeAndValidateJSONPayloadCallCount()).To(Equal(1))
			actualReq, _ := requestValidator.DecodeAndValidateJSONPayloadArgsForCall(0)
			Expect(bodyString(actualReq)).To(Equal("the-json-body"))
		})
		When("the request body is invalid", func() {
			BeforeEach(func() {
				requestValidator.DecodeAndValidateJSONPayloadReturns(apierrors.NewUnprocessableEntityError(errors.New("validation-err"), "validation error"))
			})

			It("returns an unprocessable entity error", func() {
				expectUnprocessableEntityError("validation error")
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
	})

	Describe("GET /v3/apps/:guid/routes", func() {
		BeforeEach(func() {
			routeRepo.ListRoutesForAppReturns([]repositories.RouteRecord{
				{
					GUID:     "test-route-guid",
					Host:     "test-route-host",
					Path:     "/some_path",
					Protocol: "http",
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
				GUID:  dropletGUID,
				State: "STAGED",
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

	Describe("GET /v3/apps/:guid/droplets", func() {
		BeforeEach(func() {
			dropletRepo.ListDropletsReturns(repositories.ListResult[repositories.DropletRecord]{
				PageInfo: descriptors.PageInfo{
					TotalResults: 1,
				},
				Records: []repositories.DropletRecord{{
					GUID:    dropletGUID,
					State:   "STAGED",
					AppGUID: appGUID,
				}},
			}, nil)

			req = createHttpRequest("GET", "/v3/apps/"+appGUID+"/droplets", nil)
		})

		It("returns the list of droplets", func() {
			Expect(dropletRepo.ListDropletsCallCount()).To(Equal(1))
			_, actualAuthInfo, dropletListMessage := dropletRepo.ListDropletsArgsForCall(0)
			Expect(actualAuthInfo).To(Equal(authInfo))

			Expect(dropletListMessage).To(Equal(repositories.ListDropletsMessage{
				AppGUIDs: []string{appGUID},
			}))

			Expect(rr).To(HaveHTTPStatus(http.StatusOK))
			Expect(rr).To(HaveHTTPHeaderWithValue("Content-Type", "application/json"))
			Expect(rr).To(HaveHTTPBody(SatisfyAll(
				MatchJSONPath("$.pagination.total_results", BeEquivalentTo(1)),
				MatchJSONPath("$.resources", HaveLen(1)),
				MatchJSONPath("$.resources[0].guid", Equal(dropletGUID)),
				MatchJSONPath("$.resources[0].relationships.app.data.guid", Equal(appGUID)),
				MatchJSONPath("$.resources[0].state", Equal("STAGED")),
			)))
		})

		When("the app is not accessible", func() {
			BeforeEach(func() {
				appRepo.GetAppReturns(
					repositories.AppRecord{},
					apierrors.NewForbiddenError(nil, repositories.AppResourceType),
				)
			})

			It("returns an error", func() {
				expectNotFoundError("App")
			})
		})

		When("there is some other error fetching the app", func() {
			BeforeEach(func() {
				appRepo.GetAppReturns(
					repositories.AppRecord{},
					errors.New("unknown!"),
				)
			})

			It("returns an error", func() {
				expectUnknownError()
			})
		})

		When("the droplet is not accessible", func() {
			BeforeEach(func() {
				dropletRepo.ListDropletsReturns(
					repositories.ListResult[repositories.DropletRecord]{},
					apierrors.NewForbiddenError(nil, repositories.DropletResourceType),
				)
			})

			It("returns an error", func() {
				expectNotFoundError("Droplet")
			})
		})

		When("there is some other error fetching the droplet", func() {
			BeforeEach(func() {
				dropletRepo.ListDropletsReturns(
					repositories.ListResult[repositories.DropletRecord]{},
					errors.New("unknown!"),
				)
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

	Describe("GET /v3/apps/:guid/environment_variables", func() {
		BeforeEach(func() {
			appRepo.GetAppEnvReturns(repositories.AppEnvRecord{
				AppGUID:              appGUID,
				EnvironmentVariables: map[string]string{"VAR": "VAL"},
			}, nil)

			req = createHttpRequest("GET", "/v3/apps/"+appGUID+"/environment_variables", nil)
		})

		It("returns the app environment variables", func() {
			Expect(appRepo.GetAppEnvCallCount()).To(Equal(1))
			_, actualAuthInfo, _ := appRepo.GetAppEnvArgsForCall(0)
			Expect(actualAuthInfo).To(Equal(authInfo))

			Expect(rr).To(HaveHTTPStatus(http.StatusOK))
			Expect(rr).To(HaveHTTPHeaderWithValue("Content-Type", "application/json"))
			Expect(rr).To(HaveHTTPBody(MatchJSONPath("$.var.VAR", "VAL")))
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
		var payload *payloads.AppPatchEnvVars

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
			payload = &payloads.AppPatchEnvVars{
				Var: map[string]interface{}{
					"KEY1": nil,
					"KEY2": "VAL2",
				},
			}
			requestValidator.DecodeAndValidateJSONPayloadStub = decodeAndValidatePayloadStub(payload)

			req = createHttpRequest("PATCH", "/v3/apps/"+appGUID+"/environment_variables", strings.NewReader("the-json-body"))
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

		It("validates the payload", func() {
			Expect(requestValidator.DecodeAndValidateJSONPayloadCallCount()).To(Equal(1))
			actualReq, _ := requestValidator.DecodeAndValidateJSONPayloadArgsForCall(0)
			Expect(bodyString(actualReq)).To(Equal("the-json-body"))
		})

		When("the request body is invalid", func() {
			BeforeEach(func() {
				requestValidator.DecodeAndValidateJSONPayloadReturns(apierrors.NewUnprocessableEntityError(errors.New("validation-err"), "validation error"))
			})

			It("returns an unprocessable entity error", func() {
				expectUnprocessableEntityError("validation error")
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
				CreatedAt:   time.UnixMilli(1000),
				UpdatedAt:   tools.PtrTo(time.UnixMilli(2000)),
				Labels:      map[string]string{},
				Annotations: map[string]string{},
				ImageRef:    "registry.repo/foo",
			}

			package1Record = packageRecord

			package2Record = packageRecord
			package2Record.GUID = "package-2-guid"
			package2Record.UID = "package-2-guid"
			package2Record.State = "READY"

			packageRepo.ListPackagesReturns(repositories.ListResult[repositories.PackageRecord]{
				PageInfo: descriptors.PageInfo{
					TotalResults: 2,
					TotalPages:   1,
					PageNumber:   1,
					PageSize:     2,
				},
				Records: []repositories.PackageRecord{
					package1Record,
					package2Record,
				},
			}, nil)

			req = createHttpRequest("GET", "/v3/apps/"+appGUID+"/packages", nil)
		})

		It("returns the packages", func() {
			Expect(rr).To(HaveHTTPStatus(http.StatusOK))
			Expect(rr).To(HaveHTTPHeaderWithValue("Content-Type", "application/json"))
			Expect(rr).To(HaveHTTPBody(SatisfyAll(
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
				packageRepo.ListPackagesReturns(repositories.ListResult[repositories.PackageRecord]{}, errors.New("unknown!"))
			})

			It("returns an error", func() {
				expectUnknownError()
			})
		})
	})

	Describe("GET /v3/apps/GUID/ssh_enabled", func() {
		BeforeEach(func() {
			req = createHttpRequest("GET", "/v3/apps/"+appGUID+"/ssh_enabled", nil)
		})

		It("returns false", func() {
			Expect(rr).To(HaveHTTPStatus(http.StatusOK))
			Expect(rr).To(HaveHTTPHeaderWithValue("Content-Type", "application/json"))
			Expect(rr).To(HaveHTTPBody(SatisfyAll(
				MatchJSONPath("$.enabled", BeFalse()),
				MatchJSONPath("$.reason", Equal("Disabled globally")),
			)))
		})
	})

	Describe("GET /v3/apps/GUID/features", func() {
		When("feature ssh is called", func() {
			BeforeEach(func() {
				req = createHttpRequest("GET", "/v3/apps/"+appGUID+"/features/ssh", nil)
			})

			It("returns ssh enabled false", func() {
				Expect(rr).To(HaveHTTPStatus(http.StatusOK))
				Expect(rr).To(HaveHTTPHeaderWithValue("Content-Type", "application/json"))
				Expect(rr).To(HaveHTTPBody(SatisfyAll(
					MatchJSONPath("$.name", Equal("ssh")),
					MatchJSONPath("$.description", Equal("Enable SSHing into the app.")),
					MatchJSONPath("$.enabled", BeFalse()),
				)))
			})
		})
		When("feature revisions is called", func() {
			BeforeEach(func() {
				req = createHttpRequest("GET", "/v3/apps/"+appGUID+"/features/revisions", nil)
			})

			It("returns revisions enabled false", func() {
				Expect(rr).To(HaveHTTPStatus(http.StatusOK))
				Expect(rr).To(HaveHTTPHeaderWithValue("Content-Type", "application/json"))
				Expect(rr).To(HaveHTTPBody(SatisfyAll(
					MatchJSONPath("$.name", Equal("revisions")),
					MatchJSONPath("$.description", Equal("Enable versioning of an application")),
					MatchJSONPath("$.enabled", BeFalse()),
				)))
			})
		})
		When("anything else is called", func() {
			BeforeEach(func() {
				req = createHttpRequest("GET", "/v3/apps/"+appGUID+"/features/anything-else", nil)
			})

			It("returns feature not found", func() {
				Expect(rr).To(HaveHTTPStatus(http.StatusNotFound))
				Expect(rr).To(HaveHTTPHeaderWithValue("Content-Type", "application/json"))
				Expect(rr).To(HaveHTTPBody(SatisfyAll(
					MatchJSONPath("$.errors", HaveLen(1)),
					MatchJSONPath("$.errors[0].detail", Equal("Feature not found. Ensure it exists and you have access to it.")),
					MatchJSONPath("$.errors[0].title", Equal("CF-ResourceNotFound")),
					MatchJSONPath("$.errors[0].code", BeEquivalentTo(10010)),
				)))
			})
		})
	})

	Describe("DELETE /v3/apps/:guid/processes/:process/instances/:instance", func() {
		BeforeEach(func() {
			processRepo.ListProcessesReturns([]repositories.ProcessRecord{
				{
					GUID:             "process-1-guid",
					SpaceGUID:        spaceGUID,
					AppGUID:          appGUID,
					Type:             "web",
					DesiredInstances: 1,
				},
			}, nil)
			req = createHttpRequest("DELETE", "/v3/apps/"+appGUID+"/processes/web/instances/0", nil)
		})

		It("restarts the instance", func() {
			Expect(rr).To(HaveHTTPStatus(http.StatusNoContent))
			Expect(podRepo.DeletePodCallCount()).To(Equal(1))
			_, actualAuthInfo, actualAppRevision, actualProcess, actualInstanceID := podRepo.DeletePodArgsForCall(0)
			Expect(actualAuthInfo).To(Equal(authInfo))
			Expect(actualAppRevision).To(Equal("0"))
			Expect(actualProcess.AppGUID).To(Equal(appGUID))
			Expect(actualProcess.SpaceGUID).To(Equal(spaceGUID))
			Expect(actualProcess.Type).To(Equal("web"))
			Expect(actualInstanceID).To(Equal("0"))
		})
		When("the process does not exist", func() {
			BeforeEach(func() {
				req = createHttpRequest("DELETE", "/v3/apps/"+appGUID+"/processes/boom/instances/0", nil)
			})
			It("returns an error", func() {
				expectNotFoundError("Process")
			})
		})
		When("the instance does not exist", func() {
			BeforeEach(func() {
				req = createHttpRequest("DELETE", "/v3/apps/"+appGUID+"/processes/web/instances/5", nil)
			})
			It("returns an error", func() {
				expectNotFoundError("Instance 5 of process web")
			})
		})
		When("the app has a worker process", func() {
			BeforeEach(func() {
				processRepo.ListProcessesReturns([]repositories.ProcessRecord{
					{
						GUID:             "process-1-guid",
						SpaceGUID:        spaceGUID,
						AppGUID:          appGUID,
						Type:             "web",
						DesiredInstances: 1,
					},
					{
						GUID:             "process-2-guid",
						SpaceGUID:        spaceGUID,
						AppGUID:          appGUID,
						Type:             "worker",
						DesiredInstances: 2,
					},
				}, nil)
				req = createHttpRequest("DELETE", "/v3/apps/"+appGUID+"/processes/worker/instances/0", nil)
			})

			It("restarts the instance", func() {
				Expect(rr).To(HaveHTTPStatus(http.StatusNoContent))
				Expect(podRepo.DeletePodCallCount()).To(Equal(1))
				_, actualAuthInfo, actualAppRevision, actualProcess, actualInstanceID := podRepo.DeletePodArgsForCall(0)
				Expect(actualAuthInfo).To(Equal(authInfo))
				Expect(actualAppRevision).To(Equal("0"))
				Expect(actualProcess.AppGUID).To(Equal(appGUID))
				Expect(actualProcess.SpaceGUID).To(Equal(spaceGUID))
				Expect(actualProcess.Type).To(Equal("worker"))
				Expect(actualInstanceID).To(Equal("0"))
			})
		})
		When("The app does not exist", func() {
			BeforeEach(func() {
				appRepo.GetAppReturns(repositories.AppRecord{}, errors.New("App not found"))
			})
			It("returns an error", func() {
				expectNotFoundError("App")
			})
		})
	})
})

func createHttpRequest(method string, url string, body io.Reader) *http.Request {
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	Expect(err).NotTo(HaveOccurred())

	return req
}
