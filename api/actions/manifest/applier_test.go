package manifest_test

import (
	"context"
	"errors"

	"code.cloudfoundry.org/korifi/api/actions/manifest"
	"code.cloudfoundry.org/korifi/api/actions/shared/fake"
	"code.cloudfoundry.org/korifi/api/authorization"
	apierrors "code.cloudfoundry.org/korifi/api/errors"
	"code.cloudfoundry.org/korifi/api/payloads"
	"code.cloudfoundry.org/korifi/api/repositories"
	"code.cloudfoundry.org/korifi/tools"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
)

var _ = Describe("Applier", func() {
	var (
		appRepo             *fake.CFAppRepository
		domainRepo          *fake.CFDomainRepository
		processRepo         *fake.CFProcessRepository
		routeRepo           *fake.CFRouteRepository
		serviceInstanceRepo *fake.CFServiceInstanceRepository
		serviceBindingRepo  *fake.CFServiceBindingRepository
		applier             *manifest.Applier
		applierErr          error
		ctx                 context.Context
		authInfo            authorization.Info
		appInfo             payloads.ManifestApplication
		appState            manifest.AppState
	)

	BeforeEach(func() {
		appRepo = new(fake.CFAppRepository)
		domainRepo = new(fake.CFDomainRepository)
		processRepo = new(fake.CFProcessRepository)
		routeRepo = new(fake.CFRouteRepository)
		serviceInstanceRepo = new(fake.CFServiceInstanceRepository)
		serviceBindingRepo = new(fake.CFServiceBindingRepository)
		applier = manifest.NewApplier(appRepo, domainRepo, processRepo, routeRepo, serviceInstanceRepo, serviceBindingRepo)
		ctx = context.Background()
		authInfo = authorization.Info{Token: "a-token"}
		appInfo = payloads.ManifestApplication{
			Name:       "my-app",
			Env:        map[string]string{"FOO": "bar", "BOB": "bob"},
			NoRoute:    false,
			Processes:  []payloads.ManifestApplicationProcess{},
			Routes:     []payloads.ManifestRoute{},
			Buildpacks: []string{"buildpack-a"},
			Metadata: payloads.MetadataPatch{
				Labels: map[string]*string{
					"foo":      tools.PtrTo("FOO"),
					"novalue1": tools.PtrTo(""),
					"clear1":   nil,
				},
				Annotations: map[string]*string{
					"bar":      tools.PtrTo("BAR"),
					"novalue2": tools.PtrTo(""),
					"clear2":   nil,
				},
			},
		}
		appState = manifest.AppState{
			App:       repositories.AppRecord{},
			Processes: map[string]repositories.ProcessRecord{},
			Routes:    map[string]repositories.RouteRecord{},
		}
	})

	JustBeforeEach(func() {
		applierErr = applier.Apply(ctx, authInfo, "space-guid", appInfo, appState)
	})

	Describe("applying the app", func() {
		It("creates the app", func() {
			Expect(applierErr).NotTo(HaveOccurred())
			Expect(appRepo.CreateAppCallCount()).To(Equal(1))
			_, _, createAppMsg := appRepo.CreateAppArgsForCall(0)
			Expect(createAppMsg.Name).To(Equal(appInfo.Name))
			Expect(createAppMsg.SpaceGUID).To(Equal("space-guid"))
			Expect(createAppMsg.State).To(Equal(repositories.StoppedState))
			Expect(createAppMsg.Lifecycle).To(Equal(repositories.Lifecycle{
				Type: "buildpack",
				Data: repositories.LifecycleData{
					Buildpacks: []string{"buildpack-a"},
				},
			}))
			Expect(createAppMsg.EnvironmentVariables).To(Equal(appInfo.Env))
			Expect(createAppMsg.Labels).To(Equal(map[string]string{"foo": "FOO", "novalue1": ""}))
			Expect(createAppMsg.Annotations).To(Equal(map[string]string{"bar": "BAR", "novalue2": ""}))
		})

		When("creating the app fails", func() {
			BeforeEach(func() {
				appRepo.CreateAppReturns(repositories.AppRecord{}, errors.New("create-app-failed"))
			})

			It("returns the error", func() {
				Expect(applierErr).To(MatchError("create-app-failed"))
			})
		})

		When("the app exists", func() {
			BeforeEach(func() {
				appState.App = repositories.AppRecord{
					Name:        "my-app",
					GUID:        "my-guid",
					EtcdUID:     "etcd-uid",
					SpaceGUID:   "space-guid",
					Labels:      map[string]string{"foo": "FOO'"},
					Annotations: map[string]string{"bar": "BAR'"},
				}
			})

			It("updates the app buildpacks", func() {
				Expect(applierErr).NotTo(HaveOccurred())
				Expect(appRepo.PatchAppCallCount()).To(Equal(1))
				_, _, patchAppMsg := appRepo.PatchAppArgsForCall(0)
				Expect(patchAppMsg.AppGUID).To(Equal("my-guid"))
				Expect(*patchAppMsg.Lifecycle.Data.Buildpacks).To(Equal([]string{"buildpack-a"}))

				Expect(patchAppMsg.Labels).To(MatchAllKeys(Keys{
					"foo":      PointTo(Equal("FOO")),
					"novalue1": PointTo(Equal("")),
					"clear1":   BeNil(),
				}))
				Expect(patchAppMsg.Annotations).To(MatchAllKeys(Keys{
					"bar":      PointTo(Equal("BAR")),
					"novalue2": PointTo(Equal("")),
					"clear2":   BeNil(),
				}))
			})

			When("patching the app fails", func() {
				BeforeEach(func() {
					appRepo.PatchAppReturns(repositories.AppRecord{}, errors.New("patch-app-failed"))
				})

				It("returns the error", func() {
					Expect(applierErr).To(MatchError("patch-app-failed"))
				})
			})
		})
	})

	Describe("applying processes", func() {
		BeforeEach(func() {
			appState.App.GUID = "app-guid"
			appState.App.SpaceGUID = "space-guid"
			appInfo.Processes = []payloads.ManifestApplicationProcess{
				{
					Type:                         "bob",
					Command:                      tools.PtrTo("echo foo"),
					DiskQuota:                    tools.PtrTo("512M"),
					HealthCheckHTTPEndpoint:      tools.PtrTo("foo/bar"),
					HealthCheckInvocationTimeout: tools.PtrTo(int64(10)),
					HealthCheckType:              tools.PtrTo("http"),
					Instances:                    tools.PtrTo(2),
					Memory:                       tools.PtrTo("756M"),
					Timeout:                      tools.PtrTo(int64(31)),
				},
				{
					Type:                         "ben",
					Command:                      tools.PtrTo("echo bar"),
					DiskQuota:                    tools.PtrTo("256M"),
					HealthCheckHTTPEndpoint:      tools.PtrTo("bar/foo"),
					HealthCheckInvocationTimeout: tools.PtrTo(int64(20)),
					HealthCheckType:              tools.PtrTo("port"),
					Instances:                    tools.PtrTo(3),
					Memory:                       tools.PtrTo("1024M"),
					Timeout:                      tools.PtrTo(int64(45)),
				},
			}
		})

		It("creates each process", func() {
			Expect(applierErr).NotTo(HaveOccurred())
			Expect(processRepo.PatchProcessCallCount()).To(Equal(0))
			Expect(processRepo.CreateProcessCallCount()).To(Equal(2))

			_, _, createMsg := processRepo.CreateProcessArgsForCall(0)
			Expect(createMsg.AppGUID).To(Equal("app-guid"))
			Expect(createMsg.SpaceGUID).To(Equal("space-guid"))
			Expect(createMsg.Type).To(Equal("bob"))
			Expect(createMsg.Command).To(Equal("echo foo"))
			Expect(createMsg.DiskQuotaMB).To(BeEquivalentTo(512))
			Expect(createMsg.MemoryMB).To(BeEquivalentTo(756))
			Expect(createMsg.DesiredInstances).To(PointTo(Equal(2)))
			Expect(createMsg.HealthCheck).To(Equal(repositories.HealthCheck{
				Type: "http",
				Data: repositories.HealthCheckData{
					HTTPEndpoint:             "foo/bar",
					InvocationTimeoutSeconds: 10,
					TimeoutSeconds:           31,
				},
			}))

			_, _, createMsg = processRepo.CreateProcessArgsForCall(1)
			Expect(createMsg.AppGUID).To(Equal("app-guid"))
			Expect(createMsg.SpaceGUID).To(Equal("space-guid"))
			Expect(createMsg.Type).To(Equal("ben"))
			Expect(createMsg.Command).To(Equal("echo bar"))
			Expect(createMsg.DiskQuotaMB).To(BeEquivalentTo(256))
			Expect(createMsg.MemoryMB).To(BeEquivalentTo(1024))
			Expect(createMsg.DesiredInstances).To(PointTo(Equal(3)))
			Expect(createMsg.HealthCheck).To(Equal(repositories.HealthCheck{
				Type: "port",
				Data: repositories.HealthCheckData{
					HTTPEndpoint:             "bar/foo",
					InvocationTimeoutSeconds: 20,
					TimeoutSeconds:           45,
				},
			}))
		})

		When("creating a process fails", func() {
			BeforeEach(func() {
				processRepo.CreateProcessReturns(errors.New("create-process-failed"))
			})

			It("returns the error", func() {
				Expect(applierErr).To(MatchError("create-process-failed"))
			})
		})

		When("a process exists", func() {
			BeforeEach(func() {
				appState.Processes = map[string]repositories.ProcessRecord{
					"ben": {GUID: "process-guid"},
				}
			})

			It("patches that process", func() {
				Expect(applierErr).NotTo(HaveOccurred())
				Expect(processRepo.CreateProcessCallCount()).To(Equal(1))
				Expect(processRepo.PatchProcessCallCount()).To(Equal(1))

				_, _, patchMsg := processRepo.PatchProcessArgsForCall(0)
				Expect(patchMsg.SpaceGUID).To(Equal("space-guid"))
				Expect(patchMsg.ProcessGUID).To(Equal("process-guid"))
				Expect(patchMsg.Command).To(Equal(tools.PtrTo("echo bar")))
				Expect(patchMsg.DiskQuotaMB).To(Equal(tools.PtrTo(int64(256))))
				Expect(patchMsg.MemoryMB).To(Equal(tools.PtrTo(int64(1024))))
				Expect(patchMsg.DesiredInstances).To(Equal(tools.PtrTo(3)))
				Expect(patchMsg.HealthCheckType).To(Equal(tools.PtrTo("port")))
				Expect(patchMsg.HealthCheckHTTPEndpoint).To(Equal(tools.PtrTo("bar/foo")))
				Expect(patchMsg.HealthCheckInvocationTimeoutSeconds).To(Equal(tools.PtrTo(int64(20))))
				Expect(patchMsg.HealthCheckTimeoutSeconds).To(Equal(tools.PtrTo(int64(45))))
			})

			When("patching the process fails", func() {
				BeforeEach(func() {
					processRepo.PatchProcessReturns(repositories.ProcessRecord{}, errors.New("process-patch-error"))
				})

				It("returns the error", func() {
					Expect(applierErr).To(MatchError("process-patch-error"))
				})
			})
		})
	})

	Describe("applying routes", func() {
		BeforeEach(func() {
			appState.App.GUID = "app-guid"
			appState.App.SpaceGUID = "space-guid"
			appInfo.Routes = []payloads.ManifestRoute{
				{Route: tools.PtrTo("r1.my.domain/my-path")},
			}
			domainRepo.GetDomainByNameReturns(repositories.DomainRecord{
				Namespace: "domain-namespace",
				Name:      "domain-name",
				GUID:      "domain-guid",
			}, nil)

			routeRepo.GetOrCreateRouteReturns(repositories.RouteRecord{
				GUID:      "route-guid",
				SpaceGUID: "space-guid",
				Destinations: []repositories.DestinationRecord{{
					GUID: "dest-guid",
				}},
			}, nil)
		})

		It("creates the route", func() {
			Expect(domainRepo.GetDomainByNameCallCount()).To(Equal(1))
			_, _, domainName := domainRepo.GetDomainByNameArgsForCall(0)
			Expect(domainName).To(Equal("my.domain"))

			Expect(routeRepo.GetOrCreateRouteCallCount()).To(Equal(1))
			_, _, createRouteMessage := routeRepo.GetOrCreateRouteArgsForCall(0)
			Expect(createRouteMessage).To(Equal(repositories.CreateRouteMessage{
				Host:            "r1",
				Path:            "/my-path",
				SpaceGUID:       "space-guid",
				DomainNamespace: "domain-namespace",
				DomainName:      "domain-name",
				DomainGUID:      "domain-guid",
			}))
		})

		It("adds a destination to the route", func() {
			Expect(routeRepo.AddDestinationsToRouteCallCount()).To(Equal(1))
			_, _, addDestinationMessage := routeRepo.AddDestinationsToRouteArgsForCall(0)
			Expect(addDestinationMessage).To(Equal(repositories.AddDestinationsToRouteMessage{
				RouteGUID:            "route-guid",
				SpaceGUID:            "space-guid",
				ExistingDestinations: []repositories.DestinationRecord{{GUID: "dest-guid"}},
				NewDestinations: []repositories.DestinationMessage{{
					AppGUID:     "app-guid",
					ProcessType: "web",
					Port:        8080,
					Protocol:    "http1",
				}},
			}))
		})

		When("adding the destination to the route fails", func() {
			BeforeEach(func() {
				routeRepo.AddDestinationsToRouteReturns(repositories.RouteRecord{}, errors.New("add-route-to-dest-error"))
			})
			It("returns the error", func() {
				Expect(applierErr).To(MatchError(ContainSubstring("add-route-to-dest-error")))
			})
		})

		When("getting the domain fails", func() {
			BeforeEach(func() {
				domainRepo.GetDomainByNameReturns(repositories.DomainRecord{}, errors.New("get-domain-err"))
			})

			It("returns the error", func() {
				Expect(applierErr).To(MatchError(ContainSubstring("get-domain-err")))
			})
		})

		When("get/create route fails", func() {
			BeforeEach(func() {
				routeRepo.GetOrCreateRouteReturns(repositories.RouteRecord{}, errors.New("get-create-route-err"))
			})

			It("returns the error", func() {
				Expect(applierErr).To(MatchError(ContainSubstring("get-create-route-err")))
			})
		})

		When("there are multiple routes", func() {
			BeforeEach(func() {
				appInfo.Routes = append(appInfo.Routes, payloads.ManifestRoute{Route: tools.PtrTo("r2.my.domain")})
			})

			It("creates them", func() {
				Expect(routeRepo.GetOrCreateRouteCallCount()).To(Equal(2))
			})
		})

		When("the route already exists", func() {
			BeforeEach(func() {
				appState.Routes = map[string]repositories.RouteRecord{"r1.my.domain/my-path": {}}
			})

			It("doesn't do any route creation", func() {
				Expect(domainRepo.GetDomainByNameCallCount()).To(BeZero())
				Expect(routeRepo.GetOrCreateRouteCallCount()).To(BeZero())
			})
		})

		When("the no-route is set in the manifest", func() {
			BeforeEach(func() {
				appInfo.NoRoute = true
				appState.Routes = map[string]repositories.RouteRecord{
					"r1.my.domain/my-path": {
						GUID:      "route-guid",
						SpaceGUID: "space-guid",
						Destinations: []repositories.DestinationRecord{{
							GUID:    "dest1-guid",
							AppGUID: "app-guid",
						}, {
							GUID:    "dest2-guid",
							AppGUID: "app-guid",
						}, {
							GUID:    "dest3-guid",
							AppGUID: "another-app-guid",
						}},
					},
				}
			})

			It("removes all the destinations for that app from the route", func() {
				Expect(routeRepo.RemoveDestinationFromRouteCallCount()).To(Equal(2))

				_, _, removeDest1Msg := routeRepo.RemoveDestinationFromRouteArgsForCall(0)
				Expect(removeDest1Msg).To(Equal(repositories.RemoveDestinationFromRouteMessage{
					RouteGUID:       "route-guid",
					SpaceGUID:       "space-guid",
					DestinationGuid: "dest1-guid",
				}))

				_, _, removeDest2Msg := routeRepo.RemoveDestinationFromRouteArgsForCall(1)
				Expect(removeDest2Msg).To(Equal(repositories.RemoveDestinationFromRouteMessage{
					RouteGUID:       "route-guid",
					SpaceGUID:       "space-guid",
					DestinationGuid: "dest2-guid",
				}))
			})

			When("removing destination fails", func() {
				BeforeEach(func() {
					routeRepo.RemoveDestinationFromRouteReturns(repositories.RouteRecord{}, errors.New("remove-dest-err"))
				})

				It("returns the error", func() {
					Expect(applierErr).To(MatchError(ContainSubstring("remove-dest-err")))
				})
			})

			When("the app has multiple routes", func() {
				BeforeEach(func() {
					appState.Routes = map[string]repositories.RouteRecord{
						"r1.my.domain/my-path": {
							GUID:      "route-guid",
							SpaceGUID: "space-guid",
							Destinations: []repositories.DestinationRecord{{
								GUID:    "dest1-guid",
								AppGUID: "app-guid",
							}},
						},
						"r2.my-domain/your-path": {
							GUID:      "route2-guid",
							SpaceGUID: "space-guid",
							Destinations: []repositories.DestinationRecord{{
								GUID:    "dest2-guid",
								AppGUID: "app-guid",
							}},
						},
					}
				})

				It("deletes the destinations from all routes", func() {
					Expect(routeRepo.RemoveDestinationFromRouteCallCount()).To(Equal(2))
					_, _, removeDest1Msg := routeRepo.RemoveDestinationFromRouteArgsForCall(0)
					_, _, removeDest2Msg := routeRepo.RemoveDestinationFromRouteArgsForCall(1)

					Expect([]string{
						removeDest1Msg.DestinationGuid,
						removeDest2Msg.DestinationGuid,
					}).To(ConsistOf("dest1-guid", "dest2-guid"))
				})
			})
		})
	})

	Describe("applying services", func() {
		BeforeEach(func() {
			serviceInstanceRepo.ListServiceInstancesReturns([]repositories.ServiceInstanceRecord{
				{Name: "service-name", GUID: "service-guid"},
			}, nil)

			appState.App.GUID = "app-guid"
			appState.App.SpaceGUID = "space-guid"
			appState.ServiceBindings = map[string]repositories.ServiceBindingRecord{
				"already-bound-service-name": {},
			}

			appInfo.Services = []payloads.ManifestApplicationService{
				{Name: "service-name"},
				{Name: "already-bound-service-name"},
			}
		})

		It("creates a service binding", func() {
			Expect(applierErr).NotTo(HaveOccurred())

			Expect(serviceInstanceRepo.ListServiceInstancesCallCount()).To(Equal(1))
			_, _, listMsg := serviceInstanceRepo.ListServiceInstancesArgsForCall(0)
			Expect(listMsg).To(Equal(repositories.ListServiceInstanceMessage{
				Names: []string{"service-name"},
			}))

			Expect(serviceBindingRepo.CreateServiceBindingCallCount()).To(Equal(1))
			_, _, createMsg := serviceBindingRepo.CreateServiceBindingArgsForCall(0)
			Expect(createMsg).To(Equal(repositories.CreateServiceBindingMessage{
				ServiceInstanceGUID: "service-guid",
				AppGUID:             "app-guid",
				SpaceGUID:           "space-guid",
			}))
		})

		When("the desired binding has its name specified", func() {
			BeforeEach(func() {
				appInfo.Services = []payloads.ManifestApplicationService{
					{Name: "service-name", BindingName: tools.PtrTo("service-binding")},
					{Name: "already-bound-service-name"},
				}
			})

			It("creates a named service binding", func() {
				Expect(applierErr).NotTo(HaveOccurred())

				Expect(serviceBindingRepo.CreateServiceBindingCallCount()).To(Equal(1))
				_, _, createMsg := serviceBindingRepo.CreateServiceBindingArgsForCall(0)
				Expect(createMsg).To(Equal(repositories.CreateServiceBindingMessage{
					Name:                tools.PtrTo("service-binding"),
					ServiceInstanceGUID: "service-guid",
					AppGUID:             "app-guid",
					SpaceGUID:           "space-guid",
				}))
			})
		})

		When("listing service instances fails", func() {
			BeforeEach(func() {
				serviceInstanceRepo.ListServiceInstancesReturns(nil, errors.New("list-services-err"))
			})

			It("returns the error", func() {
				Expect(applierErr).To(MatchError(ContainSubstring("list-services-err")))
			})
		})

		When("creating the service binding fails", func() {
			BeforeEach(func() {
				serviceBindingRepo.CreateServiceBindingReturns(repositories.ServiceBindingRecord{}, errors.New("create-sb-err"))
			})

			It("returns the error", func() {
				Expect(applierErr).To(MatchError(ContainSubstring("create-sb-err")))
			})
		})

		When("the app is already bound to all the services in the manifest", func() {
			BeforeEach(func() {
				appState.ServiceBindings = map[string]repositories.ServiceBindingRecord{
					"service-name":               {},
					"already-bound-service-name": {},
				}
			})

			It("does not bind the app again", func() {
				Expect(applierErr).NotTo(HaveOccurred())
				Expect(serviceBindingRepo.CreateServiceBindingCallCount()).To(BeZero())
			})
		})

		When("the manifest references service instances that do not exist", func() {
			BeforeEach(func() {
				appInfo.Services = []payloads.ManifestApplicationService{
					{Name: "unexisting-service"},
				}
			})

			It("returns a not found error error", func() {
				Expect(applierErr).To(BeAssignableToTypeOf(apierrors.NotFoundError{}))
				notFoundErr := applierErr.(apierrors.NotFoundError)
				Expect(notFoundErr.Detail()).To(ContainSubstring("unexisting-service"))

				Expect(serviceBindingRepo.CreateServiceBindingCallCount()).To(BeZero())
			})
		})
	})
})
