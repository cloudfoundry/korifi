package handlers_test

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	apierrors "code.cloudfoundry.org/korifi/api/errors"
	"code.cloudfoundry.org/korifi/api/handlers"
	"code.cloudfoundry.org/korifi/api/handlers/fake"
	"code.cloudfoundry.org/korifi/api/payloads"
	"code.cloudfoundry.org/korifi/api/repositories"
	. "code.cloudfoundry.org/korifi/tests/matchers"
	"code.cloudfoundry.org/korifi/tools"
	"github.com/google/uuid"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Build", func() {
	var (
		createdAt = time.UnixMilli(1000)
		updatedAt = tools.PtrTo(time.UnixMilli(2000))

		req              *http.Request
		requestValidator *fake.RequestValidator
		apiHandler       *handlers.Build
		appRepo          *fake.CFAppRepository
		buildRepo        *fake.CFBuildRepository
		packageRepo      *fake.CFPackageRepository
	)

	BeforeEach(func() {
		requestValidator = new(fake.RequestValidator)
		appRepo = new(fake.CFAppRepository)
		buildRepo = new(fake.CFBuildRepository)
		packageRepo = new(fake.CFPackageRepository)

		apiHandler = handlers.NewBuild(
			*serverURL,
			buildRepo,
			packageRepo,
			appRepo,
			requestValidator,
		)
		routerBuilder.LoadRoutes(apiHandler)
	})

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
		)

		BeforeEach(func() {
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
		})

		It("returns the Build", func() {
			Expect(rr).To(HaveHTTPStatus(http.StatusOK))
			Expect(rr).To(HaveHTTPHeaderWithValue("Content-Type", "application/json"))
			Expect(rr).To(HaveHTTPBody(SatisfyAll(
				MatchJSONPath("$.guid", "test-build-guid"),
				MatchJSONPath("$.state", "STAGING"),
				MatchJSONPath("$.links.self.href", HavePrefix("https://api.example.org")),
			)))
		})

		When("the user does not have access to the build", func() {
			BeforeEach(func() {
				buildRepo.GetBuildReturns(repositories.BuildRecord{}, apierrors.NewForbiddenError(nil, repositories.BuildResourceType))
			})

			It("returns an error", func() {
				expectNotFoundError("Build")
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

	Describe("the GET /v3/builds endpoint", func() {
		var buildGUID string

		BeforeEach(func() {
			buildGUID = uuid.NewString()
			buildRepo.ListBuildsReturns([]repositories.BuildRecord{
				{
					GUID:      buildGUID,
					State:     "STAGING",
					CreatedAt: createdAt,
					UpdatedAt: updatedAt,
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
			req, err = http.NewRequestWithContext(ctx, "GET", "/v3/builds", nil)
			Expect(err).NotTo(HaveOccurred())
		})

		It("returns the list of builds", func() {
			Expect(rr).To(HaveHTTPStatus(http.StatusOK))

			Expect(buildRepo.ListBuildsCallCount()).To(Equal(1))
			_, actualAuthInfo, _ := buildRepo.ListBuildsArgsForCall(0)
			Expect(actualAuthInfo).To(Equal(authInfo))

			Expect(rr).To(HaveHTTPHeaderWithValue("Content-Type", "application/json"))
			Expect(rr).To(HaveHTTPBody(SatisfyAll(
				MatchJSONPath("$.pagination.total_results", BeEquivalentTo(1)),
				MatchJSONPath("$.pagination.first.href", "https://api.example.org/v3/builds"),
				MatchJSONPath("$.resources", HaveLen(1)),
				MatchJSONPath("$.resources[0].state", "STAGING"),
				MatchJSONPath("$.resources[0].guid", buildGUID),
			)))
		})

		When("filtering query params are provided", func() {
			BeforeEach(func() {
				requestValidator.DecodeAndValidateURLValuesStub = decodeAndValidateURLValuesStub(&payloads.BuildList{
					PackageGUIDs: "p1,p2",
					AppGUIDs:     "a1,a2",
					States:       "s1,s2",
				})
			})

			It("passes them to the repository", func() {
				Expect(buildRepo.ListBuildsCallCount()).To(Equal(1))
				_, _, message := buildRepo.ListBuildsArgsForCall(0)

				Expect(message.PackageGUIDs).To(ConsistOf("p1", "p2"))
				Expect(message.AppGUIDs).To(ConsistOf("a1", "a2"))
				Expect(message.States).To(ConsistOf("s1", "s2"))
			})
		})

		When("there is some other error fetching the list of builds", func() {
			BeforeEach(func() {
				buildRepo.ListBuildsReturns([]repositories.BuildRecord{}, errors.New("unknown!"))
			})

			It("returns an error", func() {
				expectUnknownError()
			})
		})
	})

	Describe("the POST /v3/builds endpoint", func() {
		var expectedLifecycleBuildpacks []string

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
		)

		BeforeEach(func() {
			requestValidator.DecodeAndValidateJSONPayloadStub = decodeAndValidatePayloadStub(&payloads.BuildCreate{
				Package: &payloads.RelationshipData{
					GUID: packageGUID,
				},
			})

			expectedLifecycleBuildpacks = []string{"buildpack-a", "buildpack-b"}

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

			var err error
			req, err = http.NewRequestWithContext(ctx, "POST", "/v3/builds", strings.NewReader(""))
			Expect(err).NotTo(HaveOccurred())
		})

		It("creates the build", func() {
			Expect(appRepo.GetAppCallCount()).To(Equal(1))
			_, _, actualAppGUID := appRepo.GetAppArgsForCall(0)
			Expect(actualAppGUID).To(Equal(appGUID))

			Expect(buildRepo.CreateBuildCallCount()).To(Equal(1))
			_, _, actualCreate := buildRepo.CreateBuildArgsForCall(0)

			Expect(actualCreate.SpaceGUID).To(Equal(spaceGUID))
			Expect(actualCreate.AppGUID).To(Equal(appGUID))
			Expect(actualCreate.PackageGUID).To(Equal(packageGUID))
			Expect(actualCreate.StagingMemoryMB).To(Equal(expectedStagingMem))
			Expect(actualCreate.Lifecycle.Type).To(Equal(expectedLifecycleType))
			Expect(actualCreate.Lifecycle.Data.Buildpacks).To(Equal(expectedLifecycleBuildpacks))
			Expect(actualCreate.Lifecycle.Data.Stack).To(Equal(expectedLifecycleStack))

			Expect(rr).To(HaveHTTPStatus(http.StatusCreated))
			Expect(rr).To(HaveHTTPHeaderWithValue("Content-Type", "application/json"))
			Expect(rr).To(HaveHTTPBody(SatisfyAll(
				MatchJSONPath("$.guid", "test-build-guid"),
				MatchJSONPath("$.state", "STAGING"),
				MatchJSONPath("$.links.self.href", HavePrefix("https://api.example.org")),
			)))
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
				requestValidator.DecodeAndValidateJSONPayloadReturns(apierrors.NewUnprocessableEntityError(nil, "oops"))
			})

			It("returns an error", func() {
				expectUnprocessableEntityError("oops")
			})
		})
	})

	Describe("the PATCH /v3/builds endpoint", func() {
		BeforeEach(func() {
			var err error
			req, err = http.NewRequestWithContext(context.Background(), "PATCH", "/v3/builds/build-guid", strings.NewReader(`{}`))
			Expect(err).NotTo(HaveOccurred())
		})

		It("returns an unprocessable entity error", func() {
			expectUnprocessableEntityError("Labels and annotations are not supported for builds.")
		})
	})
})
