package actions_test

import (
	"context"
	"errors"
	"fmt"

	. "code.cloudfoundry.org/korifi/api/actions"
	"code.cloudfoundry.org/korifi/api/actions/fake"
	"code.cloudfoundry.org/korifi/api/apierrors"
	"code.cloudfoundry.org/korifi/api/authorization"
	"code.cloudfoundry.org/korifi/api/payloads"
	"code.cloudfoundry.org/korifi/api/repositories"
	"code.cloudfoundry.org/korifi/tests/matchers"
	"k8s.io/apimachinery/pkg/types"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("ApplyManifest", func() {
	const (
		spaceGUID         = "test-space-guid"
		appName           = "my-app"
		appGUID           = "my-app-guid"
		appEtcdUID        = types.UID("my-app-etcd-uid")
		defaultDomainName = "default-domain.com"
		defaultDomainGUID = "default-domain-guid"
		rootNamespace     = "cf"
	)
	var (
		manifest    payloads.Manifest
		appRepo     *fake.CFAppRepository
		domainRepo  *fake.CFDomainRepository
		processRepo *fake.CFProcessRepository
		routeRepo   *fake.CFRouteRepository
		authInfo    authorization.Info

		manifestAction *Manifest
		applyErr       error
	)

	BeforeEach(func() {
		appRepo = new(fake.CFAppRepository)
		domainRepo = new(fake.CFDomainRepository)
		domainRepo.GetDomainByNameReturns(repositories.DomainRecord{
			Name:      defaultDomainName,
			GUID:      defaultDomainGUID,
			Namespace: rootNamespace,
		}, nil)

		processRepo = new(fake.CFProcessRepository)
		routeRepo = new(fake.CFRouteRepository)
		authInfo = authorization.Info{Token: "a-token"}
		manifest = payloads.Manifest{
			Version: 1,
			Applications: []payloads.ManifestApplication{
				{
					Name: appName,
					Env:  map[string]string{"FOO": "bar"},
					Processes: []payloads.ManifestApplicationProcess{
						{Type: "bob"},
					},
				},
			},
		}

		manifestAction = NewManifest(appRepo, domainRepo, processRepo, routeRepo, defaultDomainName)
	})

	JustBeforeEach(func() {
		applyErr = manifestAction.Apply(context.Background(), authInfo, spaceGUID, manifest)
	})

	It("fetches the app correctly", func() {
		Expect(appRepo.GetAppByNameAndSpaceCallCount()).To(Equal(1))

		_, actualAuthInfo, actualAppName, actualSpaceGUID := appRepo.GetAppByNameAndSpaceArgsForCall(0)
		Expect(actualAuthInfo).To(Equal(authInfo))
		Expect(actualAppName).To(Equal(appName))
		Expect(actualSpaceGUID).To(Equal(spaceGUID))
	})

	When("fetching the app returns a Forbidden error", func() {
		BeforeEach(func() {
			appRepo.GetAppByNameAndSpaceReturns(repositories.AppRecord{}, apierrors.NewForbiddenError(errors.New("boom"), repositories.AppResourceType))
		})

		It("returns a NotFound error", func() {
			Expect(applyErr).To(matchers.WrapErrorAssignableToTypeOf(apierrors.NotFoundError{}))
		})

		It("doesn't create an App", func() {
			Expect(appRepo.CreateAppCallCount()).To(Equal(0))
		})
	})

	When("fetching the app returns a non-API error", func() {
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

	When("the manifest includes application resource properties, but no web process", func() {
		BeforeEach(func() {
			manifest = payloads.Manifest{
				Version: 1,
				Applications: []payloads.ManifestApplication{
					{
						Name:      appName,
						Memory:    stringPointer("128M"),
						Processes: []payloads.ManifestApplicationProcess{},
					},
				},
			}

			appRepo.GetAppByNameAndSpaceReturns(repositories.AppRecord{}, apierrors.NewNotFoundError(nil, repositories.AppResourceType))
			appRepo.CreateAppReturns(repositories.AppRecord{GUID: appGUID}, nil)
		})

		It("creates a web process with the given resource properties", func() {
			Expect(processRepo.CreateProcessCallCount()).To(Equal(1))
			_, _, processMessage := processRepo.CreateProcessArgsForCall(0)
			Expect(processMessage.AppGUID).To(Equal(appGUID))
			Expect(processMessage.Type).To(Equal("web"))
			Expect(processMessage.MemoryMB).To(Equal(int64(128)))
		})
	})

	When("the manifest includes application resource properties, and a web process", func() {
		BeforeEach(func() {
			manifest = payloads.Manifest{
				Version: 1,
				Applications: []payloads.ManifestApplication{
					{
						Name:   appName,
						Memory: stringPointer("128M"),
						Processes: []payloads.ManifestApplicationProcess{
							{
								Type:   "web",
								Memory: stringPointer("256M"),
							},
						},
					},
				},
			}

			appRepo.GetAppByNameAndSpaceReturns(repositories.AppRecord{}, apierrors.NewNotFoundError(nil, repositories.AppResourceType))
			appRepo.CreateAppReturns(repositories.AppRecord{GUID: appGUID}, nil)
		})

		It("creates a web process with the given resource properties", func() {
			Expect(processRepo.CreateProcessCallCount()).To(Equal(1))
			_, _, processMessage := processRepo.CreateProcessArgsForCall(0)
			Expect(processMessage.AppGUID).To(Equal(appGUID))
			Expect(processMessage.Type).To(Equal("web"))
			Expect(processMessage.MemoryMB).To(Equal(int64(256)))
		})
	})

	When("the app does not exist", func() {
		BeforeEach(func() {
			appRepo.GetAppByNameAndSpaceReturns(repositories.AppRecord{}, apierrors.NewNotFoundError(nil, repositories.AppResourceType))
			appRepo.CreateAppReturns(repositories.AppRecord{GUID: appGUID}, nil)
		})

		It("creates the app and its processes", func() {
			Expect(appRepo.CreateAppCallCount()).To(Equal(1))

			_, actualAuthInfo, appMessage := appRepo.CreateAppArgsForCall(0)
			Expect(actualAuthInfo).To(Equal(authInfo))
			Expect(appMessage.Name).To(Equal(appName))
			Expect(appMessage.SpaceGUID).To(Equal(spaceGUID))

			Expect(processRepo.CreateProcessCallCount()).To(Equal(1))
			_, actualAuthInfo, processMessage := processRepo.CreateProcessArgsForCall(0)
			Expect(actualAuthInfo).To(Equal(authInfo))
			Expect(processMessage.AppGUID).To(Equal(appGUID))
			Expect(processMessage.SpaceGUID).To(Equal(spaceGUID))
			Expect(processMessage.Type).To(Equal("bob"))
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
		BeforeEach(func() {
			appRepo.GetAppByNameAndSpaceReturns(repositories.AppRecord{
				Name:      appName,
				GUID:      appGUID,
				EtcdUID:   appEtcdUID,
				SpaceGUID: spaceGUID,
			}, nil)
		})

		It("updates the app env vars", func() {
			Expect(appRepo.CreateOrPatchAppEnvVarsCallCount()).To(Equal(1))

			_, actualAuthInfo, message := appRepo.CreateOrPatchAppEnvVarsArgsForCall(0)
			Expect(actualAuthInfo).To(Equal(authInfo))
			Expect(message.AppGUID).To(Equal(appGUID))
			Expect(message.SpaceGUID).To(Equal(spaceGUID))
			Expect(message.AppEtcdUID).To(Equal(appEtcdUID))
			Expect(message.EnvironmentVariables).To(Equal(map[string]string{"FOO": "bar"}))
		})

		When("updating the env vars errors", func() {
			BeforeEach(func() {
				appRepo.CreateOrPatchAppEnvVarsReturns(repositories.AppEnvVarsRecord{}, errors.New("boom"))
			})

			It("returns an error", func() {
				Expect(applyErr).To(MatchError(ContainSubstring("boom")))
			})
		})

		It("checks if the process exists correctly", func() {
			Expect(processRepo.GetProcessByAppTypeAndSpaceCallCount()).To(Equal(1))

			_, actualAuthInfo, actualAppGUID, actualProcessType, actualSpaceGUID := processRepo.GetProcessByAppTypeAndSpaceArgsForCall(0)
			Expect(actualAuthInfo).To(Equal(authInfo))
			Expect(actualAppGUID).To(Equal(appGUID))
			Expect(actualProcessType).To(Equal("bob"))
			Expect(actualSpaceGUID).To(Equal(spaceGUID))
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

			It("patches the process", func() {
				Expect(processRepo.PatchProcessCallCount()).To(Equal(1))

				_, actualAuthInfo, message := processRepo.PatchProcessArgsForCall(0)
				Expect(actualAuthInfo).To(Equal(authInfo))
				Expect(message.ProcessGUID).To(Equal("totes-real"))
				Expect(message.SpaceGUID).To(Equal(spaceGUID))
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
				processRepo.GetProcessByAppTypeAndSpaceReturns(repositories.ProcessRecord{}, apierrors.NewNotFoundError(nil, repositories.ProcessResourceType))
			})

			It("creates the process", func() {
				Expect(processRepo.CreateProcessCallCount()).To(Equal(1))

				_, actualAuthInfo, message := processRepo.CreateProcessArgsForCall(0)
				Expect(actualAuthInfo).To(Equal(authInfo))
				Expect(message.AppGUID).To(Equal(appGUID))
				Expect(message.SpaceGUID).To(Equal(spaceGUID))
				Expect(message.Type).To(Equal("bob"))
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

		When("default route is specified for the app, and no routes are specified", func() {
			BeforeEach(func() {
				manifest.Applications[0].DefaultRoute = true
			})

			It("checks for existing routes correctly", func() {
				Expect(routeRepo.ListRoutesForAppCallCount()).To(Equal(1))

				_, actualAuthInfo, actualAppGUID, actualSpaceGUID := routeRepo.ListRoutesForAppArgsForCall(0)
				Expect(actualAuthInfo).To(Equal(authInfo))
				Expect(actualAppGUID).To(Equal(appGUID))
				Expect(actualSpaceGUID).To(Equal(spaceGUID))
			})

			When("checking for existing routes fails", func() {
				BeforeEach(func() {
					routeRepo.ListRoutesForAppReturns(nil, errors.New("boom"))
				})

				It("returns the error", func() {
					Expect(applyErr).To(MatchError("boom"))
				})
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

				When("fetching the default domain fails with a NotFound error", func() {
					BeforeEach(func() {
						domainRepo.GetDomainByNameReturns(repositories.DomainRecord{}, apierrors.NewNotFoundError(errors.New("boom"), repositories.DomainResourceType))
					})

					It("returns an UnprocessibleEntity error with a friendly message", func() {
						Expect(applyErr).To(matchers.WrapErrorAssignableToTypeOf(apierrors.UnprocessableEntityError{}))
						var apierr apierrors.ApiError
						ok := errors.As(applyErr, &apierr)
						Expect(ok).To(BeTrue())
						Expect(apierr.Detail()).To(Equal(
							fmt.Sprintf("The configured default domain %q was not found", defaultDomainName),
						))
					})
				})

				When("fetching the default domain fails", func() {
					BeforeEach(func() {
						domainRepo.GetDomainByNameReturns(repositories.DomainRecord{}, errors.New("fail-on-purpose"))
					})

					It("returns an error", func() {
						Expect(applyErr).To(MatchError("fail-on-purpose"))
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

		When("no route is specified for the app", func() {
			BeforeEach(func() {
				manifest.Applications[0].NoRoute = true
			})

			When("default route is set to true", func() {
				BeforeEach(func() {
					manifest.Applications[0].DefaultRoute = true
				})

				It("does not call GetOrCreateRoute, and does not return an error", func() {
					Expect(applyErr).To(Succeed())
					Expect(routeRepo.GetOrCreateRouteCallCount()).To(Equal(0))
					Expect(routeRepo.AddDestinationsToRouteCallCount()).To(Equal(0))
				})
			})

			When("random route is set to true", func() {
				BeforeEach(func() {
					manifest.Applications[0].RandomRoute = true
				})

				It("does not call GetOrCreateRoute, and does not return an error", func() {
					Expect(applyErr).To(Succeed())
					Expect(routeRepo.GetOrCreateRouteCallCount()).To(Equal(0))
					Expect(routeRepo.AddDestinationsToRouteCallCount()).To(Equal(0))
				})
			})

			When("routes are specified in the manifest", func() {
				BeforeEach(func() {
					manifest.Applications[0].Routes = []payloads.ManifestRoute{
						{Route: stringPointer("my-app.my-domain.com/path")},
					}
				})

				It("does not call GetOrCreateRoute, and does not return an error", func() {
					Expect(applyErr).To(Succeed())
					Expect(routeRepo.GetOrCreateRouteCallCount()).To(Equal(0))
					Expect(routeRepo.AddDestinationsToRouteCallCount()).To(Equal(0))
				})
			})
		})

		When("random route is specified for the app, and no routes are specified", func() {
			BeforeEach(func() {
				manifest.Applications[0].RandomRoute = true
			})

			It("checks for existing routes correctly", func() {
				Expect(routeRepo.ListRoutesForAppCallCount()).To(Equal(1))

				_, actualAuthInfo, actualAppGUID, actualSpaceGUID := routeRepo.ListRoutesForAppArgsForCall(0)
				Expect(actualAuthInfo).To(Equal(authInfo))
				Expect(actualAppGUID).To(Equal(appGUID))
				Expect(actualSpaceGUID).To(Equal(spaceGUID))
			})

			When("checking for existing routes fails", func() {
				BeforeEach(func() {
					routeRepo.ListRoutesForAppReturns(nil, errors.New("boom"))
				})

				It("returns the error", func() {
					Expect(applyErr).To(MatchError("boom"))
				})
			})

			When("the app has no existing route destinations", func() {
				It("fetches the default domain, and calls create route for a random destination", func() {
					Expect(applyErr).To(Succeed())
					Expect(routeRepo.GetOrCreateRouteCallCount()).To(Equal(1))
					_, _, createMessage := routeRepo.GetOrCreateRouteArgsForCall(0)
					Expect(createMessage.Host).To(HavePrefix(appName + "-"))
					Expect(createMessage.Path).To(Equal(""))
					Expect(createMessage.DomainGUID).To(Equal(defaultDomainGUID))

					Expect(routeRepo.AddDestinationsToRouteCallCount()).To(Equal(1))
					_, _, destinationsMessage := routeRepo.AddDestinationsToRouteArgsForCall(0)
					Expect(destinationsMessage.NewDestinations).To(HaveLen(1))
					Expect(destinationsMessage.NewDestinations[0].AppGUID).To(Equal(appGUID))
				})

				When("fetching the default domain fails with a NotFound error", func() {
					BeforeEach(func() {
						domainRepo.GetDomainByNameReturns(repositories.DomainRecord{}, apierrors.NewNotFoundError(errors.New("boom"), repositories.DomainResourceType))
					})

					It("returns an UnprocessibleEntity error with a friendly message", func() {
						Expect(applyErr).To(matchers.WrapErrorAssignableToTypeOf(apierrors.UnprocessableEntityError{}))
						var apierr apierrors.ApiError
						ok := errors.As(applyErr, &apierr)
						Expect(ok).To(BeTrue())
						Expect(apierr.Detail()).To(Equal(
							fmt.Sprintf("The configured default domain %q was not found", defaultDomainName),
						))
					})
				})

				When("fetching the default domain fails", func() {
					BeforeEach(func() {
						domainRepo.GetDomainByNameReturns(repositories.DomainRecord{}, errors.New("fail-on-purpose"))
					})

					It("returns an error", func() {
						Expect(applyErr).To(MatchError("fail-on-purpose"))
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
				domainRepo.GetDomainByNameReturns(repositories.DomainRecord{
					Name:      "my-domain.com",
					GUID:      "my-domain-guid",
					Namespace: "my-domain-ns",
				}, nil)
				routeRepo.GetOrCreateRouteReturns(repositories.RouteRecord{
					GUID:         "route-guid",
					SpaceGUID:    spaceGUID,
					Destinations: []repositories.DestinationRecord{{GUID: "existing-destination-guid"}},
				}, nil)
				manifest.Applications[0].Routes = []payloads.ManifestRoute{
					{Route: stringPointer("my-app.my-domain.com/path")},
				}
			})

			It("creates or updates the route", func() {
				Expect(domainRepo.GetDomainByNameCallCount()).To(Equal(1))
				_, actualAuthInfo, domainName := domainRepo.GetDomainByNameArgsForCall(0)
				Expect(actualAuthInfo).To(Equal(authInfo))
				Expect(domainName).To(Equal("my-domain.com"))

				Expect(routeRepo.GetOrCreateRouteCallCount()).To(Equal(1))
				_, actualAuthInfo, getOrCreateMessage := routeRepo.GetOrCreateRouteArgsForCall(0)
				Expect(actualAuthInfo).To(Equal(authInfo))
				Expect(getOrCreateMessage.Host).To(Equal("my-app"))
				Expect(getOrCreateMessage.Path).To(Equal("/path"))
				Expect(getOrCreateMessage.SpaceGUID).To(Equal(spaceGUID))
				Expect(getOrCreateMessage.DomainGUID).To(Equal("my-domain-guid"))
				Expect(getOrCreateMessage.DomainName).To(Equal("my-domain.com"))
				Expect(getOrCreateMessage.DomainNamespace).To(Equal("my-domain-ns"))

				Expect(routeRepo.AddDestinationsToRouteCallCount()).To(Equal(1))
				_, actualAuthInfo, addDestMessage := routeRepo.AddDestinationsToRouteArgsForCall(0)
				Expect(actualAuthInfo).To(Equal(authInfo))
				Expect(addDestMessage.RouteGUID).To(Equal("route-guid"))
				Expect(addDestMessage.SpaceGUID).To(Equal(spaceGUID))
				Expect(addDestMessage.ExistingDestinations).To(ConsistOf(repositories.DestinationRecord{GUID: "existing-destination-guid"}))
				Expect(addDestMessage.NewDestinations).To(ConsistOf(repositories.DestinationMessage{
					AppGUID:     appGUID,
					ProcessType: "web",
					Port:        8080,
					Protocol:    "http1",
				}))
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

				It("is ignored, and AddDestinationsToRoute is called without adding a default destination to the existing route list", func() {
					Expect(applyErr).To(Succeed())
					Expect(routeRepo.GetOrCreateRouteCallCount()).To(Equal(len(manifest.Applications[0].Routes)))
					_, _, createMessage := routeRepo.GetOrCreateRouteArgsForCall(0)
					Expect(createMessage.Host).To(Equal("NOT-MY-APP"))
					Expect(createMessage.Path).To(Equal("/path"))
					Expect(createMessage.DomainGUID).To(Equal("my-domain-guid"))

					Expect(routeRepo.AddDestinationsToRouteCallCount()).To(Equal(1))
					_, _, destinationsMessage := routeRepo.AddDestinationsToRouteArgsForCall(0)
					Expect(destinationsMessage.NewDestinations).To(HaveLen(1))
				})
			})

			When("randomRoute:true is set on the manifest along with the routes", func() {
				BeforeEach(func() {
					manifest.Applications[0].Routes = []payloads.ManifestRoute{
						{Route: stringPointer("NOT-MY-APP.my-domain.com/path")},
					}
					manifest.Applications[0].RandomRoute = true
				})

				It("is ignored, and AddDestinationsToRoute is called without adding a random route to the existing route list", func() {
					Expect(applyErr).To(Succeed())
					Expect(routeRepo.GetOrCreateRouteCallCount()).To(Equal(len(manifest.Applications[0].Routes)))
					_, _, createMessage := routeRepo.GetOrCreateRouteArgsForCall(0)
					Expect(createMessage.Host).To(Equal("NOT-MY-APP"))
					Expect(createMessage.Path).To(Equal("/path"))
					Expect(createMessage.DomainGUID).To(Equal("my-domain-guid"))

					Expect(routeRepo.AddDestinationsToRouteCallCount()).To(Equal(1))
					_, _, destinationsMessage := routeRepo.AddDestinationsToRouteArgsForCall(0)
					Expect(destinationsMessage.NewDestinations).To(HaveLen(1))
				})
			})
		})
	})
})

func stringPointer(s string) *string {
	return &s
}
