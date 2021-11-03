package actions_test

import (
	"context"
	"errors"

	. "code.cloudfoundry.org/cf-k8s-api/actions"
	"code.cloudfoundry.org/cf-k8s-api/actions/fake"
	"code.cloudfoundry.org/cf-k8s-api/payloads"
	"code.cloudfoundry.org/cf-k8s-api/repositories"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("ApplyManifest", func() {
	const (
		spaceGUID = "test-space-guid"
		appName   = "my-app"
	)
	var (
		manifest payloads.SpaceManifestApply
		action   *ApplyManifest
		appRepo  *fake.CFAppRepository
		client   *fake.Client
	)

	BeforeEach(func() {
		appRepo = new(fake.CFAppRepository)
		client = new(fake.Client)
		action = NewApplyManifest(appRepo)
		manifest = payloads.SpaceManifestApply{
			Version: 1,
			Applications: []payloads.SpaceManifestApplyApplication{
				{Name: appName},
			},
		}
	})

	When("on the happy path", func() {
		It("fetches the App using the correct name and space", func() {
			Expect(
				action.Invoke(context.Background(), client, spaceGUID, manifest),
			).To(Succeed())

			Expect(appRepo.AppExistsWithNameAndSpaceCallCount()).To(Equal(1))
			_, _, actualAppName, actualSpaceGUID := appRepo.AppExistsWithNameAndSpaceArgsForCall(0)
			Expect(actualAppName).To(Equal(appName))
			Expect(actualSpaceGUID).To(Equal(spaceGUID))
		})

		When("the app in the manifest doesn't exist", func() {
			BeforeEach(func() {
				appRepo.AppExistsWithNameAndSpaceReturns(false, nil)
			})

			It("creates the App with the correct name", func() {
				Expect(
					action.Invoke(context.Background(), client, spaceGUID, manifest),
				).To(Succeed())

				Expect(appRepo.CreateAppCallCount()).To(Equal(1))
				_, _, appRecord := appRepo.CreateAppArgsForCall(0)
				Expect(appRecord.Name).To(Equal(appName))
				Expect(appRecord.SpaceGUID).To(Equal(spaceGUID))
			})
		})

		When("the app in the manifest already exists", func() {
			BeforeEach(func() {
				appRepo.AppExistsWithNameAndSpaceReturns(true, nil)
			})

			It("doesn't attempt to create the App", func() {
				Expect(
					action.Invoke(context.Background(), client, spaceGUID, manifest),
				).To(Succeed())

				Expect(appRepo.CreateAppCallCount()).To(Equal(0))
			})
		})
	})

	When("fetching the app errors", func() {
		BeforeEach(func() {
			appRepo.AppExistsWithNameAndSpaceReturns(false, errors.New("boom"))
		})

		It("returns an error", func() {
			Expect(
				action.Invoke(context.Background(), client, spaceGUID, manifest),
			).To(MatchError(ContainSubstring("boom")))
		})

		It("doesn't create an App", func() {
			_ = action.Invoke(context.Background(), client, spaceGUID, manifest)

			Expect(appRepo.CreateAppCallCount()).To(Equal(0))
		})
	})

	When("creating the app errors", func() {
		BeforeEach(func() {
			appRepo.CreateAppReturns(repositories.AppRecord{}, errors.New("boom"))
		})

		It("returns an error", func() {
			Expect(
				action.Invoke(context.Background(), client, spaceGUID, manifest),
			).To(MatchError(ContainSubstring("boom")))
		})
	})
})
