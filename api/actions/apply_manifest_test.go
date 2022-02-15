package actions_test

import (
	"context"
	"errors"

	. "code.cloudfoundry.org/cf-k8s-controllers/api/actions"
	"code.cloudfoundry.org/cf-k8s-controllers/api/actions/fake"
	"code.cloudfoundry.org/cf-k8s-controllers/api/authorization"
	"code.cloudfoundry.org/cf-k8s-controllers/api/payloads"
	"code.cloudfoundry.org/cf-k8s-controllers/api/repositories"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("ApplyManifest", func() {
	const (
		spaceGUID         = "test-space-guid"
		appName           = "my-app"
		appGUID           = "my-app-guid"
		defaultDomainName = "default-domain.com"
		defaultDomainGUID = "default-domain-guid"
	)
	var (
		manifest    payloads.Manifest
		appRepo     *fake.CFAppRepository
		domainRepo  *fake.CFDomainRepository
		processRepo *fake.CFProcessRepository
		routeRepo   *fake.CFRouteRepository
		authInfo    authorization.Info

		applyManifestAction *ApplyManifest
		applyErr            error
	)

	BeforeEach(func() {
		appRepo = new(fake.CFAppRepository)
		appRepo.GetAppByNameAndSpaceReturns(repositories.AppRecord{
			Name: appName,
			GUID: appGUID,
		}, nil)
		domainRepo = new(fake.CFDomainRepository)
		defaultDomainRecord := repositories.DomainRecord{
			Name: defaultDomainName,
			GUID: defaultDomainGUID,
		}
		domainRepo.GetDefaultDomainReturns(defaultDomainRecord, nil)
		domainRepo.GetDomainByNameReturns(defaultDomainRecord, nil)

		processRepo = new(fake.CFProcessRepository)
		routeRepo = new(fake.CFRouteRepository)
		authInfo = authorization.Info{Token: "a-token"}
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

		applyManifestAction = NewApplyManifest(appRepo, domainRepo, processRepo, routeRepo)
	})

	JustBeforeEach(func() {
		applyErr = applyManifestAction.Invoke(context.Background(), authInfo, spaceGUID, manifest)
	})

	When("fetching the app errors", func() {
		BeforeEach(func() {
			appRepo.GetAppByNameAndSpaceReturns(repositories.AppRecord{}, errors.New("boom"))
		})

		It("returns an error", func() {
			Expect(applyErr).To(MatchError(ContainSubstring("boom")))
		})

		It("doesn't create an App", func() {
			Expect(appRepo.CreateAppCallCount()).To(Equal(0))
		})
	})

	When("the app does not exist", func() {
		BeforeEach(func() {
			appRepo.GetAppByNameAndSpaceReturns(repositories.AppRecord{}, repositories.NewNotFoundError("App", nil))
		})

		When("creating the app errors", func() {
			BeforeEach(func() {
				appRepo.CreateAppReturns(repositories.AppRecord{}, errors.New("boom"))
			})

			It("returns an error", func() {
				Expect(applyErr).To(MatchError(ContainSubstring("boom")))
			})
		})

		When("creating a process errors", func() {
			BeforeEach(func() {
				processRepo.CreateProcessReturns(errors.New("boom"))
			})

			It("returns an error", func() {
				Expect(applyErr).To(MatchError(ContainSubstring("boom")))
			})
		})
	})

	When("the app exists", func() {
		var appRecord repositories.AppRecord

		BeforeEach(func() {
			appRecord = repositories.AppRecord{GUID: "my-app-guid", Name: appName, SpaceGUID: spaceGUID}
			appRepo.GetAppByNameAndSpaceReturns(appRecord, nil)
		})

		When("updating the env vars errors", func() {
			BeforeEach(func() {
				appRepo.CreateOrPatchAppEnvVarsReturns(repositories.AppEnvVarsRecord{}, errors.New("boom"))
			})

			It("returns an error", func() {
				Expect(applyErr).To(MatchError(ContainSubstring("boom")))
			})
		})

		When("checking if the process exists errors", func() {
			BeforeEach(func() {
				processRepo.GetProcessByAppTypeAndSpaceReturns(repositories.ProcessRecord{}, errors.New("boom"))
			})

			It("returns an error", func() {
				Expect(applyErr).To(MatchError(ContainSubstring("boom")))
			})
		})

		When("the process already exists", func() {
			BeforeEach(func() {
				processRepo.GetProcessByAppTypeAndSpaceReturns(repositories.ProcessRecord{GUID: "totes-real"}, nil)
			})

			When("patching the process errors", func() {
				BeforeEach(func() {
					processRepo.PatchProcessReturns(repositories.ProcessRecord{}, errors.New("boom"))
				})

				It("returns an error", func() {
					Expect(applyErr).To(MatchError(ContainSubstring("boom")))
				})
			})
		})

		When("the process doesn't exist", func() {
			BeforeEach(func() {
				processRepo.GetProcessByAppTypeAndSpaceReturns(repositories.ProcessRecord{}, repositories.NewNotFoundError("Process", nil))
			})

			When("creating the process errors", func() {
				BeforeEach(func() {
					processRepo.CreateProcessReturns(errors.New("boom"))
				})

				It("returns an error", func() {
					Expect(applyErr).To(MatchError(ContainSubstring("boom")))
				})
			})
		})
	})

	When("default route is specified for the app, and no routes are specified", func() {
		BeforeEach(func() {
			manifest.Applications[0].DefaultRoute = true
		})

		When("the app has no existing route destinations", func() {
			It("fetches the default domain, and calls create route for the default destination", func() {
				Expect(applyErr).To(Succeed())
				Expect(routeRepo.GetOrCreateRouteCallCount()).To(Equal(1))
				_, _, createMessage := routeRepo.GetOrCreateRouteArgsForCall(0)
				Expect(createMessage.Host).To(Equal(appName))
				Expect(createMessage.Path).To(Equal(""))
				Expect(createMessage.DomainGUID).To(Equal(defaultDomainGUID))

				Expect(routeRepo.AddDestinationsToRouteCallCount()).To(Equal(1))
				_, _, destinationsMessage := routeRepo.AddDestinationsToRouteArgsForCall(0)
				Expect(destinationsMessage.NewDestinations).To(HaveLen(1))
				Expect(destinationsMessage.NewDestinations[0].AppGUID).To(Equal(appGUID))
			})

			When("fetching the destinations for the app fails", func() {
				BeforeEach(func() {
					routeRepo.ListRoutesForAppReturns([]repositories.RouteRecord{}, errors.New("fail-on-purpose"))
				})
				It("returns an error", func() {
					Expect(applyErr).NotTo(Succeed())
				})
			})

			When("fetching the default domain fails", func() {
				BeforeEach(func() {
					domainRepo.GetDefaultDomainReturns(repositories.DomainRecord{}, errors.New("fail-on-purpose"))
				})
				It("returns an error", func() {
					Expect(applyErr).NotTo(Succeed())
				})
			})
		})
		When("the app already has a route destination", func() {
			BeforeEach(func() {
				routeRepo.ListRoutesForAppReturns([]repositories.RouteRecord{{
					GUID: "some-other-route-guid",
				}}, nil)
			})
			It("does not call GetOrCreateRoute, but does not return an error", func() {
				Expect(applyErr).To(Succeed())
				Expect(routeRepo.GetOrCreateRouteCallCount()).To(Equal(0))
				Expect(routeRepo.AddDestinationsToRouteCallCount()).To(Equal(0))
			})
		})
	})

	When("a route is specified for the app", func() {
		BeforeEach(func() {
			manifest.Applications[0].Routes = []payloads.ManifestRoute{
				{Route: stringPointer("my-app.my-domain.com/path")},
			}
		})

		When("fetching the domain errors", func() {
			BeforeEach(func() {
				domainRepo.GetDomainByNameReturns(repositories.DomainRecord{}, errors.New("boom"))
			})

			It("doesn't create the route", func() {
				Expect(routeRepo.GetOrCreateRouteCallCount()).To(Equal(0))
			})

			It("doesn't add destinations to a route", func() {
				Expect(routeRepo.AddDestinationsToRouteCallCount()).To(Equal(0))
			})

			It("errors", func() {
				Expect(applyErr).To(MatchError(ContainSubstring("boom")))
			})
		})

		When("fetching/creating the route errors", func() {
			BeforeEach(func() {
				routeRepo.GetOrCreateRouteReturns(repositories.RouteRecord{}, errors.New("boom"))
			})

			It("doesn't add destinations to a route", func() {
				Expect(routeRepo.AddDestinationsToRouteCallCount()).To(Equal(0))
			})

			It("errors", func() {
				Expect(applyErr).To(MatchError(ContainSubstring("boom")))
			})
		})

		When("adding the destination to the route errors", func() {
			BeforeEach(func() {
				routeRepo.AddDestinationsToRouteReturns(repositories.RouteRecord{}, errors.New("boom"))
			})

			It("errors", func() {
				Expect(applyErr).To(MatchError(ContainSubstring("boom")))
			})
		})

		When("defaultRoute:true is set on the manifest along with the routes", func() {
			BeforeEach(func() {
				manifest.Applications[0].Routes = []payloads.ManifestRoute{
					{Route: stringPointer("NOT-MY-APP.my-domain.com/path")},
				}
				manifest.Applications[0].DefaultRoute = true
			})
			It("is ignored, and AddDestinationsToRouteCallCount is called without adding a default destination to the existing route list", func() {
				Expect(applyErr).To(Succeed())
				Expect(routeRepo.GetOrCreateRouteCallCount()).To(Equal(len(manifest.Applications[0].Routes)))
				_, _, createMessage := routeRepo.GetOrCreateRouteArgsForCall(0)
				Expect(createMessage.Host).To(Equal("NOT-MY-APP"))
				Expect(createMessage.Path).To(Equal("/path"))
				Expect(createMessage.DomainGUID).To(Equal(defaultDomainGUID))

				Expect(routeRepo.AddDestinationsToRouteCallCount()).To(Equal(1))
				_, _, destinationsMessage := routeRepo.AddDestinationsToRouteArgsForCall(0)
				Expect(destinationsMessage.NewDestinations).To(HaveLen(1))
			})
		})
	})
})

func stringPointer(s string) *string {
	return &s
}
