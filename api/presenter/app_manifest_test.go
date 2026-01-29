package presenter_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"code.cloudfoundry.org/korifi/api/actions/manifest"
	"code.cloudfoundry.org/korifi/api/payloads"
	"code.cloudfoundry.org/korifi/api/presenter"
	"code.cloudfoundry.org/korifi/api/repositories"
	"code.cloudfoundry.org/korifi/tools"

	. "github.com/onsi/gomega/gstruct"
)

var _ = Describe("ForAppManifest", func() {
	var (
		appStateManifest payloads.ManifestApplication
		appState         manifest.AppState
	)

	BeforeEach(func() {
		appState = manifest.AppState{
			App: repositories.AppRecord{
				Name: "bob",
			},
			Processes: map[string]repositories.ProcessRecord{"web": {
				Type:             "web",
				DesiredInstances: 10,
				MemoryMB:         512,
				HealthCheck: repositories.HealthCheck{
					Type: "foo",
					Data: repositories.HealthCheckData{
						HTTPEndpoint:             "/health",
						InvocationTimeoutSeconds: 60,
						TimeoutSeconds:           20,
					},
				},
			}},

			Routes: map[string]repositories.RouteRecord{"route-url": {}},
			ServiceBindings: map[string]repositories.ServiceBindingRecord{"service-name": {
				Name:                tools.PtrTo("service-name"),
				ServiceInstanceGUID: "instance-guid",
			}},
			Droplet: &repositories.DropletRecord{
				Lifecycle: repositories.Lifecycle{
					Type: "docker",
				},
				Image: "docker-image",
			},
		}
	})

	JustBeforeEach(func() {
		appStateManifest = presenter.ForAppManifest(appState)
	})

	It("constructs app manifest", func() {
		Expect(appStateManifest).To(MatchFields(IgnoreExtras, Fields{
			"Name":   Equal("bob"),
			"Docker": HaveKeyWithValue("image", "docker-image"),
			"Processes": ContainElement(MatchFields(IgnoreExtras, Fields{
				"Type":                         Equal("web"),
				"HealthCheckInvocationTimeout": PointTo(BeEquivalentTo(60)),
				"HealthCheckType":              PointTo(Equal("foo")),
				"Instances":                    PointTo(Equal(int32(10))),
				"Memory":                       PointTo(Equal("512")),
				"Timeout":                      PointTo(Equal(int32(20))),
			})),
			"Routes": ContainElement(MatchFields(IgnoreExtras, Fields{
				"Route": PointTo(Equal("route-url")),
			})),
			"Services": ContainElement(MatchFields(IgnoreExtras, Fields{
				"Name":        Equal("instance-guid"),
				"BindingName": PointTo(Equal("service-name")),
			})),
		}))
	})
})
