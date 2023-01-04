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

		domainRepository   *reposfake.CFDomainRepository
		stateCollector     *fake.StateCollector
		normalizer         *fake.Normalizer
		applier            *fake.Applier
		appState           manifest.AppState
		normalizedManifest payloads.ManifestApplication

		appManifest payloads.Manifest
	)

	BeforeEach(func() {
		domainRepository = new(reposfake.CFDomainRepository)
		stateCollector = new(fake.StateCollector)
		normalizer = new(fake.Normalizer)
		applier = new(fake.Applier)

		appState = manifest.AppState{
			App: repositories.AppRecord{
				GUID: "app-guid",
				Name: "app-name",
			},
		}
		stateCollector.CollectStateReturns(appState, nil)

		normalizedManifest = payloads.ManifestApplication{
			Name: "app-name",
		}
		normalizer.NormalizeReturns(normalizedManifest)

		appManifest = payloads.Manifest{
			Applications: []payloads.ManifestApplication{{
				Name: "app-name",
			}},
		}

		manifestAction = actions.NewManifest(domainRepository, "my.domain", stateCollector, normalizer, applier)
	})

	JustBeforeEach(func() {
		applyErr = manifestAction.Apply(context.Background(), authorization.Info{}, "space-guid", appManifest)
	})

	It("succeeds", func() {
		Expect(applyErr).NotTo(HaveOccurred())
	})

	It("ensures the default domain is configured", func() {
		Expect(domainRepository.GetDomainByNameCallCount()).To(Equal(1))
		_, _, actualDomain := domainRepository.GetDomainByNameArgsForCall(0)
		Expect(actualDomain).To(Equal("my.domain"))
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

	It("collects the app state", func() {
		Expect(stateCollector.CollectStateCallCount()).To(Equal(1))
		_, _, actualAppName, actualSpaceGUID := stateCollector.CollectStateArgsForCall(0)
		Expect(actualAppName).To(Equal("app-name"))
		Expect(actualSpaceGUID).To(Equal("space-guid"))
	})

	When("collecting the app state fails", func() {
		BeforeEach(func() {
			stateCollector.CollectStateReturns(manifest.AppState{}, errors.New("collect-state-err"))
		})

		It("returns the error", func() {
			Expect(applyErr).To(MatchError("collect-state-err"))
		})
	})

	It("normalizes the manifest", func() {
		Expect(normalizer.NormalizeCallCount()).To(Equal(1))
		actualAppInManifest, actualState := normalizer.NormalizeArgsForCall(0)
		Expect(actualAppInManifest.Name).To(Equal("app-name"))
		Expect(actualState.App.GUID).To(Equal("app-guid"))
	})

	It("applies the normalized manifest", func() {
		Expect(applier.ApplyCallCount()).To(Equal(1))
		_, _, actualSpaceGUID, actualAppInManifest, actualState := applier.ApplyArgsForCall(0)
		Expect(actualSpaceGUID).To(Equal("space-guid"))
		Expect(actualAppInManifest.Name).To(Equal("app-name"))
		Expect(actualState.App.GUID).To(Equal("app-guid"))
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
