package manifest_test

import (
	"code.cloudfoundry.org/korifi/api/actions/manifest"
	"code.cloudfoundry.org/korifi/api/payloads"
	"code.cloudfoundry.org/korifi/api/repositories"
	"code.cloudfoundry.org/korifi/tools"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gstruct"
)

type processParams struct {
	Memory    *string
	DiskQuota *string
}

type (
	appParams       processParams
	effectiveParams processParams
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

		DescribeTable("creating a web process when top level values are provided",
			func(app appParams) {
				appInfo.Memory = app.Memory
				appInfo.DiskQuota = app.DiskQuota

				updatedAppInfo := normalizer.Normalize(appInfo, appState)
				webProc := getWebProcess(updatedAppInfo)

				Expect(webProc.Memory).To(Equal(app.Memory))
				Expect(webProc.DiskQuota).To(Equal(app.DiskQuota))
			},

			Entry("memory only", appParams{Memory: tools.PtrTo("512M")}),
			Entry("disk_quota only", appParams{DiskQuota: tools.PtrTo("2G")}),
			Entry("memory and disk_quota", appParams{Memory: tools.PtrTo("512M"), DiskQuota: tools.PtrTo("2G")}),
		)

		DescribeTable("updating a web process when top level values are provided",
			func(app appParams, process processParams, effective effectiveParams) {
				appInfo.Memory = app.Memory
				appInfo.DiskQuota = app.DiskQuota

				appInfo.Processes = append(appInfo.Processes, payloads.ManifestApplicationProcess{
					Type:      "web",
					Memory:    process.Memory,
					DiskQuota: process.DiskQuota,
				})

				updatedAppInfo := normalizer.Normalize(appInfo, appState)
				webProc := getWebProcess(updatedAppInfo)

				Expect(webProc.Memory).To(Equal(effective.Memory))
				Expect(webProc.DiskQuota).To(Equal(effective.DiskQuota))
			},

			Entry("empty proc with app memory and disk quota",
				appParams{
					Memory:    tools.PtrTo("512M"),
					DiskQuota: tools.PtrTo("2G"),
				},
				processParams{},
				effectiveParams{
					Memory:    tools.PtrTo("512M"),
					DiskQuota: tools.PtrTo("2G"),
				}),
			Entry("empty proc with app memory",
				appParams{
					Memory: tools.PtrTo("512M"),
				},
				processParams{},
				effectiveParams{
					Memory: tools.PtrTo("512M"),
				}),
			Entry("empty proc with disk quota",
				appParams{
					DiskQuota: tools.PtrTo("2G"),
				},
				processParams{},
				effectiveParams{
					DiskQuota: tools.PtrTo("2G"),
				}),
			Entry("value from proc memory used",
				appParams{
					Memory:    tools.PtrTo("256M"),
					DiskQuota: tools.PtrTo("2G"),
				},
				processParams{
					Memory: tools.PtrTo("512M"),
				},
				effectiveParams{
					Memory:    tools.PtrTo("512M"),
					DiskQuota: tools.PtrTo("2G"),
				}),
			Entry("value from proc disk_quota used",
				appParams{
					Memory:    tools.PtrTo("256M"),
					DiskQuota: tools.PtrTo("2G"),
				},
				processParams{
					DiskQuota: tools.PtrTo("3G"),
				},
				effectiveParams{
					Memory:    tools.PtrTo("256M"),
					DiskQuota: tools.PtrTo("3G"),
				}),
			Entry("values from proc memory and disk_quota used",
				appParams{
					Memory:    tools.PtrTo("256M"),
					DiskQuota: tools.PtrTo("2G"),
				},
				processParams{
					Memory:    tools.PtrTo("512M"),
					DiskQuota: tools.PtrTo("3G"),
				},
				effectiveParams{
					Memory:    tools.PtrTo("512M"),
					DiskQuota: tools.PtrTo("3G"),
				}),
		)
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

	Describe("deprecated disk-quota handling", func() {
		When("disk-quota is set on process", func() {
			BeforeEach(func() {
				appInfo.Processes = []payloads.ManifestApplicationProcess{
					{
						Type:         "bob",
						AltDiskQuota: tools.PtrTo("123M"),
					},
				}
			})

			It("sets the value to disk_quota", func() {
				Expect(normalizedAppInfo.Processes[0].DiskQuota).To(gstruct.PointTo(Equal("123M")))
			})
		})

		When("disk-quota is set on app", func() {
			BeforeEach(func() {
				//nolint:staticcheck
				appInfo.AltDiskQuota = tools.PtrTo("123M")
			})

			It("sets the value to disk_quota", func() {
				webProc := getWebProcess(normalizedAppInfo)
				Expect(webProc.DiskQuota).To(gstruct.PointTo(Equal("123M")))
			})
		})
	})
})

func getWebProcess(appInfo payloads.ManifestApplication) payloads.ManifestApplicationProcess {
	for _, proc := range appInfo.Processes {
		if proc.Type == "web" {
			return proc
		}
	}

	Fail("no web process")
	return payloads.ManifestApplicationProcess{}
}
