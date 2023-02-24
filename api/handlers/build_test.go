package handlers_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	apierrors "code.cloudfoundry.org/korifi/api/errors"
	. "code.cloudfoundry.org/korifi/api/handlers"
	"code.cloudfoundry.org/korifi/api/handlers/fake"
	"code.cloudfoundry.org/korifi/api/payloads"
	"code.cloudfoundry.org/korifi/api/repositories"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Build", func() {
	var req *http.Request

	JustBeforeEach(func() {
		routerBuilder.Build().ServeHTTP(rr, req)
	})

	Describe("the GET /v3/builds/{guid} endpoint", func() {
		const (
			appGUID     = "test-app-guid"
			packageGUID = "test-package-guid"
			buildGUID   = "test-build-guid"

			stagingMem  = 1024
			stagingDisk = 2048

			createdAt = "1906-04-18T13:12:00Z"
			updatedAt = "1906-04-18T13:12:01Z"
		)

		var buildRepo *fake.CFBuildRepository

		// set up happy path defaults
		BeforeEach(func() {
			buildRepo = new(fake.CFBuildRepository)
			buildRepo.GetBuildReturns(repositories.BuildRecord{
				GUID:            buildGUID,
				State:           "STAGING",
				CreatedAt:       createdAt,
				UpdatedAt:       updatedAt,
				StagingMemoryMB: stagingMem,
				StagingDiskMB:   stagingDisk,
				Lifecycle: repositories.Lifecycle{
					Type: "buildpack",
					Data: repositories.LifecycleData{
						Buildpacks: []string{},
						Stack:      "",
					},
				},
				PackageGUID: packageGUID,
				AppGUID:     appGUID,
			}, nil)

			var err error
			req, err = http.NewRequestWithContext(ctx, "GET", "/v3/builds/"+buildGUID, nil)
			Expect(err).NotTo(HaveOccurred())

			decoderValidator, err := NewDefaultDecoderValidator()
			Expect(err).NotTo(HaveOccurred())

			apiHandler := NewBuild(
				*serverURL,
				buildRepo,
				new(fake.CFPackageRepository),
				new(fake.CFAppRepository),
				decoderValidator,
			)
			routerBuilder.LoadRoutes(apiHandler)
		})

		When("on the happy path", func() {
			When("build staging is not complete", func() {
				It("returns status 200 OK", func() {
					Expect(rr.Code).To(Equal(http.StatusOK), "Matching HTTP response code:")
				})

				It("returns Content-Type as JSON in header", func() {
					contentTypeHeader := rr.Header().Get("Content-Type")
					Expect(contentTypeHeader).To(Equal(jsonHeader), "Matching Content-Type header:")
				})

				It("returns the Build in the response", func() {
					Expect(rr.Body.String()).To(MatchJSON(`{
					"guid": "`+buildGUID+`",
					"created_at": "`+createdAt+`",
					"updated_at": "`+updatedAt+`",
					"created_by": {},
					"state": "STAGING",
					"staging_memory_in_mb": `+fmt.Sprint(stagingMem)+`,
					"staging_disk_in_mb": `+fmt.Sprint(stagingDisk)+`,
					"error": null,
					"lifecycle": {
						"type": "buildpack",
						"data": {
							"buildpacks": [],
							"stack": ""
						}
					},
					"package": {
						"guid": "`+packageGUID+`"
					},
					"droplet": null,
					"relationships": {
						"app": {
							"data": {
								"guid": "`+appGUID+`"
							}
						}
					},
					"metadata": {
						"labels": {},
						"annotations": {}
					},
					"links": {
						"self": {
							"href": "`+defaultServerURI("/v3/builds/", buildGUID)+`"
						},
						"app": {
							"href": "`+defaultServerURI("/v3/apps/", appGUID)+`"
						}
					}
				}`), "Response body matches response:")
				})
			})

			When("build staging is successful", func() {
				BeforeEach(func() {
					buildRepo.GetBuildReturns(repositories.BuildRecord{
						GUID:            buildGUID,
						State:           "STAGED",
						CreatedAt:       createdAt,
						UpdatedAt:       updatedAt,
						StagingMemoryMB: stagingMem,
						StagingDiskMB:   stagingDisk,
						Lifecycle: repositories.Lifecycle{
							Type: "buildpack",
							Data: repositories.LifecycleData{
								Buildpacks: []string{},
								Stack:      "",
							},
						},
						PackageGUID: packageGUID,
						DropletGUID: buildGUID,
						AppGUID:     appGUID,
					}, nil)
				})

				It("returns status 200 OK", func() {
					Expect(rr.Code).To(Equal(http.StatusOK), "Matching HTTP response code:")
				})

				It("returns Content-Type as JSON in header", func() {
					contentTypeHeader := rr.Header().Get("Content-Type")
					Expect(contentTypeHeader).To(Equal(jsonHeader), "Matching Content-Type header:")
				})

				It("returns the Build in the response", func() {
					Expect(rr.Body.String()).To(MatchJSON(`{
					"guid": "`+buildGUID+`",
					"created_at": "`+createdAt+`",
					"updated_at": "`+updatedAt+`",
					"created_by": {},
					"state": "STAGED",
					"staging_memory_in_mb": `+fmt.Sprint(stagingMem)+`,
					"staging_disk_in_mb": `+fmt.Sprint(stagingDisk)+`,
					"error": null,
					"lifecycle": {
						"type": "buildpack",
						"data": {
							"buildpacks": [],
							"stack": ""
						}
					},
					"package": {
						"guid": "`+packageGUID+`"
					},
					"droplet": {
						"guid": "`+buildGUID+`"
					},
					"relationships": {
						"app": {
							"data": {
								"guid": "`+appGUID+`"
							}
						}
					},
					"metadata": {
						"labels": {},
						"annotations": {}
					},
					"links": {
						"self": {
							"href": "`+defaultServerURI("/v3/builds/", buildGUID)+`"
						},
						"app": {
							"href": "`+defaultServerURI("/v3/apps/", appGUID)+`"
						},
						"droplet": {
							"href": "`+defaultServerURI("/v3/droplets/", buildGUID)+`"
						}
					}
				}`), "Response body matches response:")
				})
			})

			When("build staging fails", func() {
				const (
					stagingErrorMsg = "StagingError: something went wrong during staging"
				)
				BeforeEach(func() {
					buildRepo.GetBuildReturns(repositories.BuildRecord{
						GUID:            buildGUID,
						State:           "FAILED",
						CreatedAt:       createdAt,
						UpdatedAt:       updatedAt,
						StagingErrorMsg: stagingErrorMsg,
						StagingMemoryMB: stagingMem,
						StagingDiskMB:   stagingDisk,
						Lifecycle: repositories.Lifecycle{
							Type: "buildpack",
							Data: repositories.LifecycleData{
								Buildpacks: []string{},
								Stack:      "",
							},
						},
						PackageGUID: packageGUID,
						DropletGUID: "",
						AppGUID:     appGUID,
					}, nil)
				})

				It("returns status 200 OK", func() {
					Expect(rr.Code).To(Equal(http.StatusOK), "Matching HTTP response code:")
				})

				It("returns Content-Type as JSON in header", func() {
					contentTypeHeader := rr.Header().Get("Content-Type")
					Expect(contentTypeHeader).To(Equal(jsonHeader), "Matching Content-Type header:")
				})

				It("returns the Build in the response", func() {
					Expect(rr.Body.String()).To(MatchJSON(`{
					"guid": "`+buildGUID+`",
					"created_at": "`+createdAt+`",
					"updated_at": "`+updatedAt+`",
					"created_by": {},
					"state": "FAILED",
					"staging_memory_in_mb": `+fmt.Sprint(stagingMem)+`,
					"staging_disk_in_mb": `+fmt.Sprint(stagingDisk)+`,
					"error": "`+stagingErrorMsg+`",
					"lifecycle": {
						"type": "buildpack",
						"data": {
							"buildpacks": [],
							"stack": ""
						}
					},
					"package": {
						"guid": "`+packageGUID+`"
					},
					"droplet": null,
					"relationships": {
						"app": {
							"data": {
								"guid": "`+appGUID+`"
							}
						}
					},
					"metadata": {
						"labels": {},
						"annotations": {}
					},
					"links": {
						"self": {
							"href": "`+defaultServerURI("/v3/builds/", buildGUID)+`"
						},
						"app": {
							"href": "`+defaultServerURI("/v3/apps/", appGUID)+`"
						}
					}
				}`), "Make sure there is no droplet and error is surfaced from record")
				})
			})
		})

		When("the user does not have access to the build", func() {
			BeforeEach(func() {
				buildRepo.GetBuildReturns(repositories.BuildRecord{}, apierrors.NewForbiddenError(nil, repositories.BuildResourceType))
			})

			It("returns an error", func() {
				expectNotFoundError("Build not found")
			})
		})

		When("there is some other error fetching the build", func() {
			BeforeEach(func() {
				buildRepo.GetBuildReturns(repositories.BuildRecord{}, errors.New("unknown!"))
			})

			It("returns an error", func() {
				expectUnknownError()
			})
		})
	})

	Describe("the POST /v3/builds endpoint", func() {
		var (
			packageRepo                 *fake.CFPackageRepository
			appRepo                     *fake.CFAppRepository
			buildRepo                   *fake.CFBuildRepository
			requestJSONValidator        *fake.RequestJSONValidator
			expectedLifecycleBuildpacks []string
			payload                     payloads.BuildCreate
		)

		const (
			packageGUID = "the-package-guid"
			packageUID  = "the-package-uid"
			appGUID     = "the-app-guid"
			buildGUID   = "test-build-guid"

			expectedStagingMem     = 1024
			expectedStagingDisk    = 1024
			expectedLifecycleType  = "buildpack"
			expectedLifecycleStack = "cflinuxfs3d"
			spaceGUID              = "the-space-guid"
			createdAt              = "1906-04-18T13:12:00Z"
			updatedAt              = "1906-04-18T13:12:01Z"
		)

		BeforeEach(func() {
			payload = payloads.BuildCreate{
				Package: &payloads.RelationshipData{
					GUID: packageGUID,
				},
			}

			requestJSONValidator = new(fake.RequestJSONValidator)
			requestJSONValidator.DecodeAndValidateJSONPayloadStub = func(_ *http.Request, i interface{}) error {
				build, ok := i.(*payloads.BuildCreate)
				Expect(ok).To(BeTrue())
				*build = payload

				return nil
			}

			expectedLifecycleBuildpacks = []string{"buildpack-a", "buildpack-b"}

			packageRepo = new(fake.CFPackageRepository)
			packageRepo.GetPackageReturns(repositories.PackageRecord{
				Type:      "bits",
				AppGUID:   appGUID,
				SpaceGUID: spaceGUID,
				GUID:      packageGUID,
				UID:       packageUID,
				State:     "READY",
				CreatedAt: createdAt,
				UpdatedAt: updatedAt,
			}, nil)

			appRepo = new(fake.CFAppRepository)
			appRepo.GetAppReturns(repositories.AppRecord{
				GUID:      appGUID,
				SpaceGUID: spaceGUID,
				Lifecycle: repositories.Lifecycle{
					Type: expectedLifecycleType,
					Data: repositories.LifecycleData{
						Buildpacks: expectedLifecycleBuildpacks,
						Stack:      expectedLifecycleStack,
					},
				},
			}, nil)

			buildRepo = new(fake.CFBuildRepository)
			buildRepo.CreateBuildReturns(repositories.BuildRecord{
				GUID:            buildGUID,
				State:           "STAGING",
				CreatedAt:       createdAt,
				UpdatedAt:       updatedAt,
				StagingMemoryMB: expectedStagingMem,
				StagingDiskMB:   expectedStagingDisk,
				Lifecycle: repositories.Lifecycle{
					Type: expectedLifecycleType,
					Data: repositories.LifecycleData{
						Buildpacks: expectedLifecycleBuildpacks,
						Stack:      expectedLifecycleStack,
					},
				},
				PackageGUID: packageGUID,
				AppGUID:     appGUID,
			}, nil)

			apiHandler := NewBuild(
				*serverURL,
				buildRepo,
				packageRepo,
				appRepo,
				requestJSONValidator,
			)
			routerBuilder.LoadRoutes(apiHandler)

			var err error
			req, err = http.NewRequestWithContext(ctx, "POST", "/v3/builds", strings.NewReader(""))
			Expect(err).NotTo(HaveOccurred())
		})

		When("on the happy path", func() {
			It("returns status 201", func() {
				Expect(rr.Code).To(Equal(http.StatusCreated), "Matching HTTP response code:")
			})

			It("returns Content-Type as JSON in header", func() {
				contentTypeHeader := rr.Header().Get("Content-Type")
				Expect(contentTypeHeader).To(Equal(jsonHeader), "Matching Content-Type header:")
			})

			It("calls create build with the correct payload", func() {
				Expect(buildRepo.CreateBuildCallCount()).To(Equal(1))
				_, _, actualCreate := buildRepo.CreateBuildArgsForCall(0)

				Expect(actualCreate.SpaceGUID).To(Equal(spaceGUID))
				Expect(actualCreate.AppGUID).To(Equal(appGUID))
				Expect(actualCreate.PackageGUID).To(Equal(packageGUID))
				Expect(actualCreate.StagingMemoryMB).To(Equal(expectedStagingMem))
				Expect(actualCreate.StagingDiskMB).To(Equal(expectedStagingDisk))
				Expect(actualCreate.Lifecycle.Type).To(Equal(expectedLifecycleType))
				Expect(actualCreate.Lifecycle.Data.Buildpacks).To(Equal(expectedLifecycleBuildpacks))
				Expect(actualCreate.Lifecycle.Data.Stack).To(Equal(expectedLifecycleStack))
			})

			It("returns the Build in the response", func() {
				Expect(rr.Body.String()).To(MatchJSON(`{
					"guid": "`+buildGUID+`",
					"created_at": "`+createdAt+`",
					"updated_at": "`+updatedAt+`",
					"created_by": {},
					"state": "STAGING",
					"staging_memory_in_mb": `+fmt.Sprint(expectedStagingMem)+`,
					"staging_disk_in_mb": `+fmt.Sprint(expectedStagingDisk)+`,
					"error": null,
					"lifecycle": {
						"type": "`+expectedLifecycleType+`",
						"data": {
							"buildpacks": ["`+expectedLifecycleBuildpacks[0]+`", "`+expectedLifecycleBuildpacks[1]+`"],
							"stack": "`+expectedLifecycleStack+`"
						}
					},
					"package": {
						"guid": "`+packageGUID+`"
					},
					"droplet": null,
					"relationships": {
						"app": {
							"data": {
								"guid": "`+appGUID+`"
							}
						}
					},
					"metadata": {
						"labels": {},
						"annotations": {}
					},
					"links": {
						"self": {
							"href": "`+defaultServerURI("/v3/builds/", buildGUID)+`"
						},
						"app": {
							"href": "`+defaultServerURI("/v3/apps/", appGUID)+`"
						}
					}
				}`), "Response body matches response:")
			})

			It("looks up the app by the correct GUID", func() {
				Expect(appRepo.GetAppCallCount()).To(Equal(1))
				_, _, actualAppGUID := appRepo.GetAppArgsForCall(0)
				Expect(actualAppGUID).To(Equal(appGUID))
			})
		})

		When("the package doesn't exist", func() {
			BeforeEach(func() {
				packageRepo.GetPackageReturns(repositories.PackageRecord{}, apierrors.NewNotFoundError(nil, repositories.PackageResourceType))
			})

			It("returns an error", func() {
				expectUnprocessableEntityError("Unable to use package. Ensure that the package exists and you have access to it.")
				Expect(buildRepo.CreateBuildCallCount()).To(Equal(0))
			})
		})

		When("the package is forbidden", func() {
			BeforeEach(func() {
				packageRepo.GetPackageReturns(repositories.PackageRecord{}, apierrors.NewForbiddenError(nil, repositories.PackageResourceType))
			})

			It("returns an error", func() {
				expectUnprocessableEntityError("Unable to use package. Ensure that the package exists and you have access to it.")
				Expect(buildRepo.CreateBuildCallCount()).To(Equal(0))
			})
		})

		When("the package exists check returns an error", func() {
			BeforeEach(func() {
				packageRepo.GetPackageReturns(repositories.PackageRecord{}, errors.New("boom"))
			})

			It("returns an error", func() {
				expectUnknownError()
				Expect(buildRepo.CreateBuildCallCount()).To(Equal(0))
			})
		})

		When("the app doesn't exist", func() {
			BeforeEach(func() {
				appRepo.GetAppReturns(repositories.AppRecord{}, apierrors.NewNotFoundError(nil, repositories.AppResourceType))
			})

			It("returns an error", func() {
				expectUnprocessableEntityError("Unable to use the app associated with that package. Ensure that the app exists and you have access to it.")
				Expect(buildRepo.CreateBuildCallCount()).To(Equal(0))
			})
		})

		When("the app is forbidden", func() {
			BeforeEach(func() {
				appRepo.GetAppReturns(repositories.AppRecord{}, apierrors.NewForbiddenError(nil, repositories.AppResourceType))
			})

			It("returns an error", func() {
				expectUnprocessableEntityError("Unable to use the app associated with that package. Ensure that the app exists and you have access to it.")
				Expect(buildRepo.CreateBuildCallCount()).To(Equal(0))
			})
		})

		When("the app exists check returns an error", func() {
			BeforeEach(func() {
				appRepo.GetAppReturns(repositories.AppRecord{}, errors.New("boom"))
			})

			It("returns an error", func() {
				expectUnknownError()
				Expect(buildRepo.CreateBuildCallCount()).To(Equal(0))
			})
		})

		When("creating the build in the repo errors", func() {
			BeforeEach(func() {
				buildRepo.CreateBuildReturns(repositories.BuildRecord{}, errors.New("boom"))
			})

			It("returns an error", func() {
				expectUnknownError()
			})
		})

		When("the JSON body is invalid", func() {
			BeforeEach(func() {
				requestJSONValidator.DecodeAndValidateJSONPayloadReturns(apierrors.NewUnprocessableEntityError(nil, "oops"))
			})

			It("returns an error", func() {
				expectUnprocessableEntityError("oops")
			})
		})
	})

	Describe("the PATCH /v3/builds endpoint", func() {
		BeforeEach(func() {
			decoderValidator, err := NewDefaultDecoderValidator()
			Expect(err).NotTo(HaveOccurred())

			apiHandler := NewBuild(
				*serverURL,
				new(fake.CFBuildRepository),
				new(fake.CFPackageRepository),
				new(fake.CFAppRepository),
				decoderValidator,
			)
			routerBuilder.LoadRoutes(apiHandler)

			req, err = http.NewRequestWithContext(context.Background(), "PATCH", "/v3/builds/build-guid", strings.NewReader(`{}`))
			Expect(err).NotTo(HaveOccurred())
		})

		It("returns an unprocessable entity error", func() {
			expectUnprocessableEntityError(`Labels and annotations are not supported for builds.`)
		})
	})
})
