package manifest_test

import (
	"context"
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"code.cloudfoundry.org/korifi/api/actions/manifest"
	"code.cloudfoundry.org/korifi/api/actions/shared/fake"
	"code.cloudfoundry.org/korifi/api/authorization"
	"code.cloudfoundry.org/korifi/api/repositories"
	"code.cloudfoundry.org/korifi/api/repositories/k8sklient/descriptors"

	apierrors "code.cloudfoundry.org/korifi/api/errors"
)

var _ = Describe("StateCollector", func() {
	var (
		appRepo             *fake.CFAppRepository
		domainRepo          *fake.CFDomainRepository
		processRepo         *fake.CFProcessRepository
		routeRepo           *fake.CFRouteRepository
		serviceInstanceRepo *fake.CFServiceInstanceRepository
		serviceBindingRepo  *fake.CFServiceBindingRepository
		dropletRepo         *fake.CFDropletRepository
		stateCollector      manifest.StateCollector
		appState            manifest.AppState
		collectStateErr     error
	)

	BeforeEach(func() {
		appRepo = new(fake.CFAppRepository)
		domainRepo = new(fake.CFDomainRepository)
		processRepo = new(fake.CFProcessRepository)
		routeRepo = new(fake.CFRouteRepository)
		serviceInstanceRepo = new(fake.CFServiceInstanceRepository)
		serviceBindingRepo = new(fake.CFServiceBindingRepository)
		dropletRepo = new(fake.CFDropletRepository)
		stateCollector = manifest.NewStateCollector(
			appRepo,
			domainRepo,
			processRepo,
			routeRepo,
			serviceInstanceRepo,
			serviceBindingRepo,
			dropletRepo,
		)
	})

	JustBeforeEach(func() {
		appState, collectStateErr = stateCollector.CollectState(context.Background(), authorization.Info{}, "app-name", "space-guid")
	})

	Describe("app", func() {
		BeforeEach(func() {
			appRepo.ListAppsReturns(repositories.ListResult[repositories.AppRecord]{Records: []repositories.AppRecord{{
				Name:      "bob",
				GUID:      "app-guid",
				EtcdUID:   "etcd-guid",
				SpaceGUID: "space-guid",
			}}}, nil)
		})

		It("sets the app record in the state", func() {
			Expect(collectStateErr).NotTo(HaveOccurred())
			Expect(appState.App.Name).To(Equal("bob"))
			Expect(appState.App.GUID).To(Equal("app-guid"))
			Expect(appState.App.EtcdUID).To(BeEquivalentTo("etcd-guid"))
			Expect(appState.App.SpaceGUID).To(Equal("space-guid"))
		})

		When("the app does not exist", func() {
			BeforeEach(func() {
				appRepo.ListAppsReturns(repositories.ListResult[repositories.AppRecord]{}, nil)
			})

			It("returns an empty app", func() {
				Expect(collectStateErr).NotTo(HaveOccurred())
				Expect(appState.App).To(Equal(repositories.AppRecord{}))
				Expect(appState.Processes).To(BeEmpty())
				Expect(appState.Routes).To(BeEmpty())
			})
		})

		When("getting the app fails", func() {
			BeforeEach(func() {
				appRepo.ListAppsReturns(repositories.ListResult[repositories.AppRecord]{}, errors.New("get-app-err"))
			})

			It("returns the error", func() {
				Expect(collectStateErr).To(MatchError("get-app-err"))
			})
		})
	})

	Describe("droplet", func() {
		BeforeEach(func() {
			appRepo.ListAppsReturns(repositories.ListResult[repositories.AppRecord]{Records: []repositories.AppRecord{{GUID: "app-guid", DropletGUID: "droplet-guid"}}}, nil)
			dropletRepo.GetDropletReturns(repositories.DropletRecord{
				Lifecycle: repositories.Lifecycle{
					Type: "buildpack",
					Data: repositories.LifecycleData{
						Buildpacks: []string{"bp1"},
					},
				},
				AppGUID: "app-guid",
				Image:   "docker-image",
			}, nil)
		})

		It("gets the droplet", func() {
			Expect(collectStateErr).NotTo(HaveOccurred())
			Expect(dropletRepo.GetDropletCallCount()).To(Equal(1))
			_, _, dropletGUID := dropletRepo.GetDropletArgsForCall(0)
			Expect(dropletGUID).To(Equal("droplet-guid"))
		})

		It("returns the droplet", func() {
			Expect(collectStateErr).NotTo(HaveOccurred())
			Expect(appState.Droplet).To(Equal(&repositories.DropletRecord{
				Lifecycle: repositories.Lifecycle{
					Type: "buildpack",
					Data: repositories.LifecycleData{
						Buildpacks: []string{"bp1"},
					},
				},
				AppGUID: "app-guid",
				Image:   "docker-image",
			}))
		})

		When("droplet is not found", func() {
			BeforeEach(func() {
				dropletRepo.GetDropletReturns(repositories.DropletRecord{}, apierrors.NewNotFoundError(nil, repositories.DropletResourceType, "droplet-guid"))
			})

			It("returns nil", func() {
				Expect(collectStateErr).NotTo(HaveOccurred())
				Expect(appState.Droplet).To(BeNil())
			})
		})

		When("getting droplet fails", func() {
			BeforeEach(func() {
				dropletRepo.GetDropletReturns(repositories.DropletRecord{}, errors.New("get-droplet-error"))
			})

			It("returns the error", func() {
				Expect(collectStateErr).To(MatchError("get-droplet-error"))
			})
		})
	})

	Describe("processes", func() {
		BeforeEach(func() {
			appRepo.ListAppsReturns(repositories.ListResult[repositories.AppRecord]{Records: []repositories.AppRecord{{GUID: "app-guid"}}}, nil)
		})

		It("lists processes", func() {
			Expect(processRepo.ListProcessesCallCount()).To(Equal(1))
			_, _, listMsg := processRepo.ListProcessesArgsForCall(0)
			Expect(listMsg.AppGUIDs).To(ConsistOf("app-guid"))
			Expect(listMsg.SpaceGUIDs).To(ConsistOf("space-guid"))
		})

		It("returns an empty map of processes", func() {
			Expect(collectStateErr).NotTo(HaveOccurred())
			Expect(appState.Processes).To(BeEmpty())
		})

		When("there are existing processes", func() {
			BeforeEach(func() {
				processRepo.ListProcessesReturns(repositories.ListResult[repositories.ProcessRecord]{
					PageInfo: descriptors.PageInfo{
						TotalResults: 2,
						TotalPages:   1,
						PageNumber:   1,
						PageSize:     2,
					},
					Records: []repositories.ProcessRecord{
						{GUID: "bob-guid", Type: "bob"},
						{GUID: "foo-guid", Type: "foo"},
					},
				}, nil)
			})

			It("constructs the process map using process type", func() {
				Expect(collectStateErr).NotTo(HaveOccurred())
				Expect(appState.Processes).To(Equal(map[string]repositories.ProcessRecord{
					"bob": {GUID: "bob-guid", Type: "bob"},
					"foo": {GUID: "foo-guid", Type: "foo"},
				}))
			})
		})

		When("list processes fails", func() {
			BeforeEach(func() {
				processRepo.ListProcessesReturns(repositories.ListResult[repositories.ProcessRecord]{}, errors.New("list-process-error"))
			})

			It("returns the error", func() {
				Expect(collectStateErr).To(MatchError("list-process-error"))
			})
		})
	})

	Describe("routes", func() {
		var routes repositories.ListResult[repositories.RouteRecord]

		BeforeEach(func() {
			appRepo.ListAppsReturns(repositories.ListResult[repositories.AppRecord]{Records: []repositories.AppRecord{{GUID: "app-guid"}}}, nil)
			routes = repositories.ListResult[repositories.RouteRecord]{
				Records: []repositories.RouteRecord{
					{
						Domain: repositories.DomainRecord{
							Name: "my.domain",
						},
						Host: "my-host",
						Path: "/my-path/foo",
					},
					{
						Domain: repositories.DomainRecord{
							Name: "my.domain",
						},
						Host: "another-host",
					},
				},
			}

			routeRepo.ListRoutesReturns(routes, nil)
		})

		It("lists the app routes", func() {
			Expect(routeRepo.ListRoutesCallCount()).To(Equal(1))
			_, _, listMsg := routeRepo.ListRoutesArgsForCall(0)
			Expect(listMsg.AppGUIDs).To(ConsistOf("app-guid"))
			Expect(listMsg.SpaceGUIDs).To(ConsistOf("space-guid"))
		})

		When("listing the routes fails", func() {
			BeforeEach(func() {
				routeRepo.ListRoutesReturns(repositories.ListResult[repositories.RouteRecord]{}, errors.New("list-routes-error"))
			})

			It("returns the error", func() {
				Expect(collectStateErr).To(MatchError("list-routes-error"))
			})
		})

		It("populates the routes map", func() {
			Expect(collectStateErr).ToNot(HaveOccurred())
			Expect(appState.Routes).To(Equal(map[string]repositories.RouteRecord{
				"my-host.my.domain/my-path/foo": routes.Records[0],
				"another-host.my.domain":        routes.Records[1],
			}))
		})
	})

	Describe("services", func() {
		var serviceBindings repositories.ListResult[repositories.ServiceBindingRecord]

		BeforeEach(func() {
			appRepo.ListAppsReturns(repositories.ListResult[repositories.AppRecord]{Records: []repositories.AppRecord{{GUID: "app-guid"}}}, nil)
			serviceInstanceRepo.ListServiceInstancesReturns(
				repositories.ListResult[repositories.ServiceInstanceRecord]{
					PageInfo: descriptors.PageInfo{TotalResults: 1},
					Records:  []repositories.ServiceInstanceRecord{{Name: "service-name", GUID: "s-guid"}},
				}, nil)

			serviceBindings = repositories.ListResult[repositories.ServiceBindingRecord]{
				PageInfo: descriptors.PageInfo{
					TotalResults: 2,
					TotalPages:   1,
					PageNumber:   1,
					PageSize:     2,
				},
				Records: []repositories.ServiceBindingRecord{
					{GUID: "sb1-guid", ServiceInstanceGUID: "s-guid"},
					{GUID: "sb2-guid", ServiceInstanceGUID: "s-guid"},
				},
			}
			serviceBindingRepo.ListServiceBindingsReturns(serviceBindings, nil)
		})

		It("lists the services for the service bindings", func() {
			Expect(serviceInstanceRepo.ListServiceInstancesCallCount()).To(Equal(1))
			_, _, listMsg := serviceInstanceRepo.ListServiceInstancesArgsForCall(0)
			Expect(listMsg.GUIDs).To(ConsistOf("s-guid"))
		})

		When("listing the services fails", func() {
			BeforeEach(func() {
				serviceInstanceRepo.ListServiceInstancesReturns(repositories.ListResult[repositories.ServiceInstanceRecord]{}, errors.New("list-service-err"))
			})

			It("returns the error", func() {
				Expect(collectStateErr).To(MatchError("list-service-err"))
			})
		})

		It("lists the app service bindings", func() {
			Expect(serviceBindingRepo.ListServiceBindingsCallCount()).To(Equal(1))
			_, _, listMessage := serviceBindingRepo.ListServiceBindingsArgsForCall(0)
			Expect(listMessage.AppGUIDs).To(ConsistOf("app-guid"))
		})

		When("listing the service bindings fails", func() {
			BeforeEach(func() {
				serviceBindingRepo.ListServiceBindingsReturns(repositories.ListResult[repositories.ServiceBindingRecord]{}, errors.New("list-sb-error"))
			})

			It("returns the error", func() {
				Expect(collectStateErr).To(MatchError("list-sb-error"))
			})
		})

		When("the service instance cannot be found for a binding", func() {
			BeforeEach(func() {
				serviceInstanceRepo.ListServiceInstancesReturns(repositories.ListResult[repositories.ServiceInstanceRecord]{
					PageInfo: descriptors.PageInfo{TotalResults: 1},
					Records:  []repositories.ServiceInstanceRecord{{Name: "service-name", GUID: "wrong-guid"}},
				}, nil)
			})

			It("returns an error", func() {
				Expect(collectStateErr).To(MatchError(ContainSubstring("no service instance found")))
			})
		})

		It("populates the service bindings map", func() {
			Expect(collectStateErr).ToNot(HaveOccurred())
			Expect(appState.ServiceBindings).To(Equal(map[string]repositories.ServiceBindingRecord{
				"service-name": serviceBindings.Records[1],
			}))
		})
	})
})
