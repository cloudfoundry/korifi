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
		manifest  payloads.SpaceManifestApply
		action    func(context.Context, client.Client, string, payloads.SpaceManifestApply) error
		appRepo   *fake.CFAppRepository
		createApp *fake.CreateAppFunc
		k8sClient *fake.Client
	)

	BeforeEach(func() {
		appRepo = new(fake.CFAppRepository)
		createApp = new(fake.CreateAppFunc)
		k8sClient = new(fake.Client)
		action = NewApplyManifest(appRepo, createApp.Spy).Invoke
		manifest = payloads.SpaceManifestApply{
			Version: 1,
			Applications: []payloads.SpaceManifestApplyApplication{
				{Name: appName},
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

			Expect(createApp.CallCount()).To(Equal(0))
		})
	})

	When("the app does not exist", func() {
		BeforeEach(func() {
			appRepo.FetchAppByNameAndSpaceReturns(repositories.AppRecord{}, repositories.NotFoundError{ResourceType: "App"})
		})

		When("creating the app errors", func() {
			BeforeEach(func() {
				createApp.Returns(repositories.AppRecord{}, errors.New("boom"))
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
	})
})
