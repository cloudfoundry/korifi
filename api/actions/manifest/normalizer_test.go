package manifest_test

import (
	"code.cloudfoundry.org/korifi/api/actions/manifest"
	"code.cloudfoundry.org/korifi/api/payloads"
	"code.cloudfoundry.org/korifi/api/repositories"
	"code.cloudfoundry.org/korifi/tools"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Normalizer", func() {
	var (
		normalizer        manifest.Normalizer
		defaultDomainName string
		appInfo           payloads.ManifestApplication
		appState          manifest.AppState

		normalizedAppInfo payloads.ManifestApplication
	)

	BeforeEach(func() {
		defaultDomainName = "my.domain"
		appInfo = payloads.ManifestApplication{
			Name:       "my-app",
			Env:        map[string]string{"FOO": "bar"},
			Buildpacks: []string{"buildpack-one", "buildpack-two"},
		}
		appState = manifest.AppState{
			App:       repositories.AppRecord{},
			Processes: nil,
			Routes:    nil,
		}
		normalizer = manifest.NewNormalizer(defaultDomainName)
	})

	JustBeforeEach(func() {
		normalizedAppInfo = normalizer.Normalize(appInfo, appState)
	})

	Describe("app normalization", func() {
		It("preserves the necessary app fields", func() {
			Expect(normalizedAppInfo.Name).To(Equal(appInfo.Name))
			Expect(normalizedAppInfo.NoRoute).To(Equal(appInfo.NoRoute))
			Expect(normalizedAppInfo.Env).To(Equal(appInfo.Env))
			Expect(normalizedAppInfo.Buildpacks).To(Equal(appInfo.Buildpacks))
		})

		When("no-route is set", func() {
			BeforeEach(func() {
				appInfo.NoRoute = true
			})

			It("propagates it", func() {
				Expect(normalizedAppInfo.NoRoute).To(BeTrue())
			})
		})
	})

	Describe("process normalization", func() {
		BeforeEach(func() {
			appInfo.Processes = []payloads.ManifestApplicationProcess{
				{Type: "bob"},
			}
		})

		It("preserves existing processes", func() {
			Expect(normalizedAppInfo.Processes).To(ConsistOf(payloads.ManifestApplicationProcess{Type: "bob"}))
		})

		When("the app has a top-level memory value set", func() {
			BeforeEach(func() {
				appInfo.Memory = tools.PtrTo("256M")
			})

			It("creates a `web` process with that memory value", func() {
				Expect(normalizedAppInfo.Processes).To(ContainElement(payloads.ManifestApplicationProcess{
					Type:   "web",
					Memory: tools.PtrTo("256M"),
				}))
			})

			When("the `web` process already exists", func() {
				BeforeEach(func() {
					appInfo.Processes = append(appInfo.Processes, payloads.ManifestApplicationProcess{
						Type:    "web",
						Command: tools.PtrTo("foo"),
					})
				})

				It("updates it", func() {
					Expect(normalizedAppInfo.Processes).To(ContainElement(payloads.ManifestApplicationProcess{
						Type:    "web",
						Command: tools.PtrTo("foo"),
						Memory:  tools.PtrTo("256M"),
					}))
				})

				When("the `web` process already sets its memory", func() {
					BeforeEach(func() {
						appInfo.Processes[len(appInfo.Processes)-1].Memory = tools.PtrTo("512M")
					})

					It("does not override it", func() {
						Expect(normalizedAppInfo.Processes).To(ContainElement(payloads.ManifestApplicationProcess{
							Type:    "web",
							Command: tools.PtrTo("foo"),
							Memory:  tools.PtrTo("512M"),
						}))
					})
				})
			})
		})
	})

	Describe("route normalization", func() {
		When("default route is set", func() {
			BeforeEach(func() {
				appInfo.DefaultRoute = true
			})

			It("creates a default route", func() {
				Expect(normalizedAppInfo.Routes).To(ConsistOf(
					payloads.ManifestRoute{
						Route: tools.PtrTo("my-app.my.domain"),
					}),
				)
			})

			When("there is already a route in the manifest", func() {
				BeforeEach(func() {
					appInfo.Routes = []payloads.ManifestRoute{{
						Route: tools.PtrTo("bob"),
					}}
				})

				It("does not add a default route", func() {
					Expect(normalizedAppInfo.Routes).To(ConsistOf(
						payloads.ManifestRoute{
							Route: tools.PtrTo("bob"),
						}),
					)
				})
			})

			When("there is already a route resource in the state", func() {
				BeforeEach(func() {
					appState.Routes = map[string]repositories.RouteRecord{
						"bob": {Host: "bob"},
					}
				})

				It("does not add a default route", func() {
					Expect(normalizedAppInfo.Routes).To(BeEmpty())
				})
			})
		})

		When("random route is set", func() {
			BeforeEach(func() {
				appInfo.RandomRoute = true
			})

			It("creates a random route", func() {
				Expect(normalizedAppInfo.Routes).To(HaveLen(1))
			})

			When("there is already a route in the manifest", func() {
				BeforeEach(func() {
					appInfo.Routes = []payloads.ManifestRoute{{
						Route: tools.PtrTo("bob"),
					}}
				})

				It("does not add a random route", func() {
					Expect(normalizedAppInfo.Routes).To(ConsistOf(
						payloads.ManifestRoute{
							Route: tools.PtrTo("bob"),
						}),
					)
				})
			})

			When("there is already a route resource in the state", func() {
				BeforeEach(func() {
					appState.Routes = map[string]repositories.RouteRecord{
						"bob": {Host: "bob"},
					}
				})

				It("does not add a random route", func() {
					Expect(normalizedAppInfo.Routes).To(BeEmpty())
				})
			})
		})
	})
})
