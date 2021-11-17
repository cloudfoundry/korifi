package actions_test

import (
	"context"
	"errors"

	"sigs.k8s.io/controller-runtime/pkg/client"

	. "code.cloudfoundry.org/cf-k8s-controllers/api/actions"
	"code.cloudfoundry.org/cf-k8s-controllers/api/actions/fake"
	"code.cloudfoundry.org/cf-k8s-controllers/api/payloads"
	"code.cloudfoundry.org/cf-k8s-controllers/api/repositories"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("ApplyManifest", func() {
	const (
		spaceGUID = "test-space-guid"
		appName   = "my-app"
	)
	var (
		manifest    payloads.Manifest
		action      func(context.Context, client.Client, string, payloads.Manifest) error
		appRepo     *fake.CFAppRepository
		processRepo *fake.CFProcessRepository
		k8sClient   *fake.Client
	)

	BeforeEach(func() {
		appRepo = new(fake.CFAppRepository)
		processRepo = new(fake.CFProcessRepository)
		k8sClient = new(fake.Client)
		action = NewApplyManifest(appRepo, processRepo).Invoke
		manifest = payloads.Manifest{
			Version: 1,
			Applications: []payloads.ManifestApplication{
				{
					Name: appName,
					Processes: []payloads.ManifestApplicationProcess{
						{Type: "bob"},
					},
				},
			},
		}
	})

	When("fetching the app errors", func() {
		BeforeEach(func() {
			appRepo.FetchAppByNameAndSpaceReturns(repositories.AppRecord{}, errors.New("boom"))
		})

		It("returns an error", func() {
			Expect(
				action(context.Background(), k8sClient, spaceGUID, manifest),
			).To(MatchError(ContainSubstring("boom")))
		})

		It("doesn't create an App", func() {
			_ = action(context.Background(), k8sClient, spaceGUID, manifest)

			Expect(appRepo.CreateAppCallCount()).To(Equal(0))
		})
	})

	When("the app does not exist", func() {
		BeforeEach(func() {
			appRepo.FetchAppByNameAndSpaceReturns(repositories.AppRecord{}, repositories.NotFoundError{ResourceType: "App"})
		})

		When("creating the app errors", func() {
			BeforeEach(func() {
				appRepo.CreateAppReturns(repositories.AppRecord{}, errors.New("boom"))
			})

			It("returns an error", func() {
				Expect(
					action(context.Background(), k8sClient, spaceGUID, manifest),
				).To(MatchError(ContainSubstring("boom")))
			})
		})

		When("creating a process errors", func() {
			BeforeEach(func() {
				processRepo.CreateProcessReturns(errors.New("boom"))
			})

			It("returns an error", func() {
				Expect(
					action(context.Background(), k8sClient, spaceGUID, manifest),
				).To(MatchError(ContainSubstring("boom")))
			})
		})
	})

	When("the app exists", func() {
		var appRecord repositories.AppRecord

		BeforeEach(func() {
			appRecord = repositories.AppRecord{GUID: "my-app-guid", Name: appName, SpaceGUID: spaceGUID}
			appRepo.FetchAppByNameAndSpaceReturns(appRecord, nil)
		})

		When("updating the env vars errors", func() {
			BeforeEach(func() {
				appRepo.CreateOrPatchAppEnvVarsReturns(repositories.AppEnvVarsRecord{}, errors.New("boom"))
			})

			It("returns an error", func() {
				Expect(
					action(context.Background(), k8sClient, spaceGUID, manifest),
				).To(MatchError(ContainSubstring("boom")))
			})
		})

		When("checking if the process exists errors", func() {
			BeforeEach(func() {
				processRepo.FetchProcessByAppTypeAndSpaceReturns(repositories.ProcessRecord{}, errors.New("boom"))
			})

			It("returns an error", func() {
				Expect(
					action(context.Background(), k8sClient, spaceGUID, manifest),
				).To(MatchError(ContainSubstring("boom")))
			})
		})

		When("the process already exists", func() {
			BeforeEach(func() {
				processRepo.FetchProcessByAppTypeAndSpaceReturns(repositories.ProcessRecord{GUID: "totes-real"}, nil)
			})

			When("patching the process errors", func() {
				BeforeEach(func() {
					processRepo.PatchProcessReturns(errors.New("boom"))
				})

				It("returns an error", func() {
					Expect(
						action(context.Background(), k8sClient, spaceGUID, manifest),
					).To(MatchError(ContainSubstring("boom")))
				})
			})
		})

		When("the process doesn't exist", func() {
			BeforeEach(func() {
				processRepo.FetchProcessByAppTypeAndSpaceReturns(repositories.ProcessRecord{}, repositories.NotFoundError{ResourceType: "Process"})
			})

			When("creating the process errors", func() {
				BeforeEach(func() {
					processRepo.CreateProcessReturns(errors.New("boom"))
				})

				It("returns an error", func() {
					Expect(
						action(context.Background(), k8sClient, spaceGUID, manifest),
					).To(MatchError(ContainSubstring("boom")))
				})
			})
		})
	})
})
