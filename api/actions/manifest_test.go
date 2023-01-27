package actions_test

import (
	"context"
	"errors"

	"code.cloudfoundry.org/korifi/api/actions"
	"code.cloudfoundry.org/korifi/api/actions/fake"
	"code.cloudfoundry.org/korifi/api/actions/manifest"
	reposfake "code.cloudfoundry.org/korifi/api/actions/shared/fake"
	"code.cloudfoundry.org/korifi/api/authorization"
	apierrors "code.cloudfoundry.org/korifi/api/errors"
	"code.cloudfoundry.org/korifi/api/payloads"
	"code.cloudfoundry.org/korifi/api/repositories"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("ApplyManifest", func() {
	var (
		manifestAction *actions.Manifest
		applyErr       error

		domainRepository *reposfake.CFDomainRepository
		stateCollector   *fake.StateCollector
		normalizer       *fake.Normalizer
		applier          *fake.Applier

		appManifest payloads.Manifest
	)

	BeforeEach(func() {
		domainRepository = new(reposfake.CFDomainRepository)
		stateCollector = new(fake.StateCollector)
		normalizer = new(fake.Normalizer)
		applier = new(fake.Applier)

		stateCollector.CollectStateReturnsOnCall(0, manifest.AppState{
			App: repositories.AppRecord{
				GUID: "app1-guid",
				Name: "app1",
			},
		}, nil)
		stateCollector.CollectStateReturnsOnCall(1, manifest.AppState{
			App: repositories.AppRecord{
				GUID: "app2-guid",
				Name: "app2",
			},
		}, nil)

		normalizer.NormalizeReturnsOnCall(0, payloads.ManifestApplication{
			Name: "normalized-app1",
		})
		normalizer.NormalizeReturnsOnCall(1, payloads.ManifestApplication{
			Name: "normalized-app2",
		})

		appManifest = payloads.Manifest{
			Applications: []payloads.ManifestApplication{{
				Name: "app1",
			}, {
				Name: "app2",
			}},
		}

		manifestAction = actions.NewManifest(domainRepository, "my.domain", stateCollector, normalizer, applier)
	})

	JustBeforeEach(func() {
		applyErr = manifestAction.Apply(context.Background(), authorization.Info{}, "space-guid", appManifest)
	})

	It("normalizes the manifest and then applies it", func() {
		Expect(applyErr).NotTo(HaveOccurred())

		Expect(domainRepository.GetDomainByNameCallCount()).To(Equal(1))
		_, _, actualDomain := domainRepository.GetDomainByNameArgsForCall(0)
		Expect(actualDomain).To(Equal("my.domain"))

		Expect(stateCollector.CollectStateCallCount()).To(Equal(2))
		_, _, actualAppName, actualSpaceGUID := stateCollector.CollectStateArgsForCall(0)
		Expect(actualAppName).To(Equal("app1"))
		Expect(actualSpaceGUID).To(Equal("space-guid"))
		_, _, actualAppName, actualSpaceGUID = stateCollector.CollectStateArgsForCall(1)
		Expect(actualAppName).To(Equal("app2"))
		Expect(actualSpaceGUID).To(Equal("space-guid"))

		Expect(normalizer.NormalizeCallCount()).To(Equal(2))
		actualAppInManifest, actualState := normalizer.NormalizeArgsForCall(0)
		Expect(actualAppInManifest.Name).To(Equal("app1"))
		Expect(actualState.App.GUID).To(Equal("app1-guid"))
		actualAppInManifest, actualState = normalizer.NormalizeArgsForCall(1)
		Expect(actualAppInManifest.Name).To(Equal("app2"))
		Expect(actualState.App.GUID).To(Equal("app2-guid"))

		Expect(applier.ApplyCallCount()).To(Equal(2))
		_, _, actualSpaceGUID, actualAppInManifest, actualState = applier.ApplyArgsForCall(0)
		Expect(actualSpaceGUID).To(Equal("space-guid"))
		Expect(actualAppInManifest.Name).To(Equal("normalized-app1"))
		Expect(actualState.App.GUID).To(Equal("app1-guid"))
		_, _, actualSpaceGUID, actualAppInManifest, actualState = applier.ApplyArgsForCall(1)
		Expect(actualSpaceGUID).To(Equal("space-guid"))
		Expect(actualAppInManifest.Name).To(Equal("normalized-app2"))
		Expect(actualState.App.GUID).To(Equal("app2-guid"))
	})

	When("the default domain does not exist", func() {
		BeforeEach(func() {
			domainRepository.GetDomainByNameReturns(repositories.DomainRecord{}, apierrors.NewNotFoundError(nil, "domain"))
		})

		It("returns an unprocessable entity error", func() {
			Expect(applyErr).To(BeAssignableToTypeOf(apierrors.UnprocessableEntityError{}))
		})
	})

	When("getting the default domain fails", func() {
		BeforeEach(func() {
			domainRepository.GetDomainByNameReturns(repositories.DomainRecord{}, errors.New("get-domain-err"))
		})

		It("returns the error", func() {
			Expect(applyErr).To(MatchError("get-domain-err"))
		})
	})

	When("collecting the app state fails", func() {
		BeforeEach(func() {
			stateCollector.CollectStateReturnsOnCall(0, manifest.AppState{}, errors.New("collect-state-err"))
		})

		It("returns the error", func() {
			Expect(applyErr).To(MatchError("collect-state-err"))
		})
	})

	When("applying the normalized manifest fails", func() {
		BeforeEach(func() {
			applier.ApplyReturns(errors.New("apply-err"))
		})

		It("returns the error", func() {
			Expect(applyErr).To(MatchError("apply-err"))
		})
	})
})
