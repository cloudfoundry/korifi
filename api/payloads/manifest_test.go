package payloads_test

import (
	. "code.cloudfoundry.org/korifi/api/payloads"
	"code.cloudfoundry.org/korifi/api/payloads/validators"
	"code.cloudfoundry.org/korifi/api/repositories"
	"code.cloudfoundry.org/korifi/tools"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
)

var _ = Describe("Manifest payload", func() {
	const spaceGUID = "the-space-guid"

	var (
		validator         validators.Manifest
		testSpaceManifest Manifest
		validateErr       error
	)

	BeforeEach(func() {
		validator = validators.NewManifest()

		testSpaceManifest = Manifest{
			Applications: []ManifestApplication{{
				Name:         "test-app",
				DefaultRoute: true,
				Memory:       nil,
				DiskQuota:    nil,
				Metadata:     MetadataPatch{},
			}},
		}
	})

	JustBeforeEach(func() {
		validateErr = validator.ValidatePayload(testSpaceManifest)
	})

	Describe("Manifest", func() {
		Describe("Validate", func() {
			It("validates the struct", func() {
				Expect(validateErr).NotTo(HaveOccurred())
			})

			When("Name is empty", func() {
				BeforeEach(func() {
					testSpaceManifest.Applications[0].Memory = tools.PtrTo("badmemory")
				})

				It("returns a validation error", func() {
					Expect(validateErr).To(MatchError("applications: (0: (memory: must use a supported unit (B, K, KB, M, MB, G, GB, T, or TB).).)."))
				})
			})
		})
	})

	Describe("ManifestApplication", func() {
		var testManifest ManifestApplication

		Describe("Validate", func() {
			BeforeEach(func() {
				testManifest = ManifestApplication{
					Name:         "test-app",
					DefaultRoute: true,
				}
				testSpaceManifest.Applications = []ManifestApplication{testManifest}
			})

			It("validates the struct", func() {
				Expect(validateErr).NotTo(HaveOccurred())
			})

			When("Name is empty", func() {
				BeforeEach(func() {
					testSpaceManifest.Applications[0].Name = ""
				})

				It("returns a validation error", func() {
					Expect(validateErr).To(MatchError(ContainSubstring("name: cannot be blank.")))
				})
			})

			When("Instances is negative", func() {
				BeforeEach(func() {
					testSpaceManifest.Applications[0].Instances = tools.PtrTo(-1)
				})
				It("returns a validation error", func() {
					Expect(validateErr).To(MatchError(ContainSubstring("instances: must be no less than 0.")))
				})
			})

			When("the disk quota doesn't supply a unit", func() {
				BeforeEach(func() {
					testSpaceManifest.Applications[0].DiskQuota = tools.PtrTo("1024")
				})

				It("returns a validation error", func() {
					Expect(validateErr).To(MatchError(ContainSubstring("disk_quota: must use a supported unit (B, K, KB, M, MB, G, GB, T, or TB).")))
				})
			})

			When("the disk quota is not positive", func() {
				BeforeEach(func() {
					testSpaceManifest.Applications[0].DiskQuota = tools.PtrTo("0MB")
				})

				It("returns a validation error", func() {
					Expect(validateErr).To(MatchError(ContainSubstring("disk_quota: must be greater than 0MB.")))
				})
			})

			When("the alt disk quota doesn't supply a unit", func() {
				BeforeEach(func() {
					testSpaceManifest.Applications[0].AltDiskQuota = tools.PtrTo("1024")
				})

				It("returns a validation error", func() {
					Expect(validateErr).To(MatchError(ContainSubstring("disk-quota: must use a supported unit (B, K, KB, M, MB, G, GB, T, or TB).")))
				})
			})

			When("the alt disk quota is not positive", func() {
				BeforeEach(func() {
					testSpaceManifest.Applications[0].AltDiskQuota = tools.PtrTo("0MB")
				})

				It("returns a validation error", func() {
					Expect(validateErr).To(MatchError(ContainSubstring("disk-quota: must be greater than 0MB.")))
				})
			})

			When("app disk-quota and app disk_quota are both set", func() {
				BeforeEach(func() {
					testSpaceManifest.Applications[0].DiskQuota = tools.PtrTo("128M")
					testSpaceManifest.Applications[0].AltDiskQuota = tools.PtrTo("128M")
				})

				It("response with an unprocessable entity error", func() {
					Expect(validateErr).To(MatchError(ContainSubstring("disk_quota: and disk-quota may not be used together.")))
				})
			})

			When("HealthCheckInvocationTimeout is not positive", func() {
				BeforeEach(func() {
					testSpaceManifest.Applications[0].HealthCheckInvocationTimeout = tools.PtrTo(int64(0))
				})
				It("returns a validation error", func() {
					Expect(validateErr).To(MatchError(ContainSubstring("health-check-invocation-timeout: must be no less than 1.")))
				})
			})

			When("HealthCheckType is invalid", func() {
				BeforeEach(func() {
					testSpaceManifest.Applications[0].HealthCheckType = tools.PtrTo("FakeHealthcheckType")
				})
				It("returns a validation error", func() {
					Expect(validateErr).To(MatchError(ContainSubstring("health-check-type: must be a valid value.")))
				})
			})

			When("Timeout is not positive", func() {
				BeforeEach(func() {
					testSpaceManifest.Applications[0].Timeout = tools.PtrTo(int64(0))
				})
				It("returns a validation error", func() {
					Expect(validateErr).To(MatchError(ContainSubstring("timeout: must be no less than 1.")))
				})
			})

			When("Memory units not valid", func() {
				BeforeEach(func() {
					testSpaceManifest.Applications[0].Memory = tools.PtrTo("5CUPS")
				})
				It("returns a validation error", func() {
					Expect(validateErr).To(MatchError(ContainSubstring("memory: must use a supported unit (B, K, KB, M, MB, G, GB, T, or TB).")))
				})
			})

			When("the memory is not positive", func() {
				BeforeEach(func() {
					testSpaceManifest.Applications[0].Memory = tools.PtrTo("0MB")
				})

				It("returns a validation error", func() {
					Expect(validateErr).To(MatchError(ContainSubstring("memory: must be greater than 0MB.")))
				})
			})

			When("random-route and default-route flags are both set", func() {
				BeforeEach(func() {
					testSpaceManifest.Applications[0].DefaultRoute = true
					testSpaceManifest.Applications[0].RandomRoute = true
				})

				It("response with an unprocessable entity error", func() {
					Expect(validateErr).To(MatchError(ContainSubstring("default-route: and random-route may not be used together.")))
				})
			})
		})
	})

	Describe("ManifestApplicationProcess", func() {
		Describe("Validate", func() {
			BeforeEach(func() {
				testSpaceManifest.Applications[0].Processes = []ManifestApplicationProcess{
					{Type: "some-type"},
				}
			})

			It("Validates the struct", func() {
				Expect(validateErr).NotTo(HaveOccurred())
			})

			When("the type is empty", func() {
				BeforeEach(func() {
					testSpaceManifest.Applications[0].Processes[0].Type = ""
				})

				It("returns a validation error", func() {
					Expect(validateErr).To(MatchError(ContainSubstring("type: cannot be blank.")))
				})
			})

			When("the disk quota doesn't supply a unit", func() {
				BeforeEach(func() {
					testSpaceManifest.Applications[0].Processes[0].DiskQuota = tools.PtrTo("1024")
				})

				It("returns a validation error", func() {
					Expect(validateErr).To(MatchError(ContainSubstring("disk_quota: must use a supported unit (B, K, KB, M, MB, G, GB, T, or TB).")))
				})
			})

			When("the disk quota is not positive", func() {
				BeforeEach(func() {
					testSpaceManifest.Applications[0].Processes[0].DiskQuota = tools.PtrTo("0MB")
				})

				It("returns a validation error", func() {
					Expect(validateErr).To(MatchError(ContainSubstring("disk_quota: must be greater than 0MB.")))
				})
			})

			When("the alt disk quota doesn't supply a unit", func() {
				BeforeEach(func() {
					testSpaceManifest.Applications[0].Processes[0].AltDiskQuota = tools.PtrTo("1024")
				})

				It("returns a validation error", func() {
					Expect(validateErr).To(MatchError(ContainSubstring("disk-quota: must use a supported unit (B, K, KB, M, MB, G, GB, T, or TB).")))
				})
			})

			When("the alt disk quota is not positive", func() {
				BeforeEach(func() {
					testSpaceManifest.Applications[0].Processes[0].AltDiskQuota = tools.PtrTo("0MB")
				})

				It("returns a validation error", func() {
					Expect(validateErr).To(MatchError(ContainSubstring("disk-quota: must be greater than 0MB.")))
				})
			})

			When("app disk-quota and app disk_quota are both set", func() {
				BeforeEach(func() {
					testSpaceManifest.Applications[0].Processes[0].DiskQuota = tools.PtrTo("128M")
					testSpaceManifest.Applications[0].Processes[0].AltDiskQuota = tools.PtrTo("128M")
				})

				It("response with an unprocessable entity error", func() {
					Expect(validateErr).To(MatchError(ContainSubstring("disk_quota: and disk-quota may not be used together.")))
				})
			})

			When("HealthCheckInvocationTimeout is not positive", func() {
				BeforeEach(func() {
					testSpaceManifest.Applications[0].Processes[0].HealthCheckInvocationTimeout = tools.PtrTo(int64(0))
				})
				It("returns a validation error", func() {
					Expect(validateErr).To(MatchError(ContainSubstring("health-check-invocation-timeout: must be no less than 1.")))
				})
			})

			When("HealthCheckType is invalid", func() {
				BeforeEach(func() {
					testSpaceManifest.Applications[0].Processes[0].HealthCheckType = tools.PtrTo("FakeHealthcheckType")
				})
				It("returns a validation error", func() {
					Expect(validateErr).To(MatchError(ContainSubstring("health-check-type: must be a valid value.")))
				})
			})

			When("Instances is negative", func() {
				BeforeEach(func() {
					testSpaceManifest.Applications[0].Processes[0].Instances = tools.PtrTo(-1)
				})
				It("returns a validation error", func() {
					Expect(validateErr).To(MatchError(ContainSubstring("instances: must be no less than 0.")))
				})
			})

			When("the memory doesn't supply a unit", func() {
				BeforeEach(func() {
					testSpaceManifest.Applications[0].Processes[0].Memory = tools.PtrTo("1024")
				})

				It("returns a validation error", func() {
					Expect(validateErr).To(MatchError(ContainSubstring("memory: must use a supported unit (B, K, KB, M, MB, G, GB, T, or TB).")))
				})
			})

			When("the memory is not positive", func() {
				BeforeEach(func() {
					testSpaceManifest.Applications[0].Processes[0].Memory = tools.PtrTo("0MB")
				})

				It("returns a validation error", func() {
					Expect(validateErr).To(MatchError(ContainSubstring("memory: must be greater than 0MB.")))
				})
			})

			When("Timeout is not positive", func() {
				BeforeEach(func() {
					testSpaceManifest.Applications[0].Processes[0].Timeout = tools.PtrTo(int64(0))
				})

				It("returns a validation error", func() {
					Expect(validateErr).To(MatchError(ContainSubstring("timeout: must be no less than 1.")))
				})
			})
		})

		Describe("ToProcessCreateMessage", func() {
			const appGUID = "the-app-guid"
			var processInfo ManifestApplicationProcess

			When("all fields are specified", func() {
				BeforeEach(func() {
					processInfo = ManifestApplicationProcess{
						Type:                         "web",
						Command:                      tools.PtrTo("start-web.sh"),
						DiskQuota:                    tools.PtrTo("512M"),
						HealthCheckHTTPEndpoint:      tools.PtrTo("/stuff"),
						HealthCheckInvocationTimeout: tools.PtrTo(int64(90)),
						HealthCheckType:              tools.PtrTo("http"),
						Instances:                    tools.PtrTo(3),
						Memory:                       tools.PtrTo("1G"),
						Timeout:                      tools.PtrTo(int64(60)),
					}
				})

				It("returns a CreateProcessMessage with those values", func() {
					message := processInfo.ToProcessCreateMessage(appGUID, spaceGUID)

					Expect(message).To(Equal(repositories.CreateProcessMessage{
						AppGUID:     appGUID,
						SpaceGUID:   spaceGUID,
						Type:        "web",
						Command:     "start-web.sh",
						DiskQuotaMB: 512,
						HealthCheck: repositories.HealthCheck{
							Type: "http",
							Data: repositories.HealthCheckData{
								HTTPEndpoint:             "/stuff",
								TimeoutSeconds:           60,
								InvocationTimeoutSeconds: 90,
							},
						},
						DesiredInstances: tools.PtrTo(3),
						MemoryMB:         1024,
					}))
				})

				Describe("HealthCheckType", func() {
					When("HealthCheckType is 'none' (legacy alias for 'process')", func() {
						const noneHealthCheckType = "none"

						It("converts the type to 'process'", func() {
							processInfo.HealthCheckType = tools.PtrTo(noneHealthCheckType)

							message := processInfo.ToProcessCreateMessage(appGUID, spaceGUID)

							Expect(message.HealthCheck.Type).To(Equal("process"))
						})
					})

					When("HealthCheckType is specified as some other valid type", func() {
						It("passes the type through to the message", func() {
							processInfo.HealthCheckType = tools.PtrTo("port")

							message := processInfo.ToProcessCreateMessage(appGUID, spaceGUID)

							Expect(message.HealthCheck.Type).To(Equal("port"))
						})
					})
				})
			})

			When("only type is specified", func() {
				BeforeEach(func() {
					processInfo = ManifestApplicationProcess{}
					processInfo.Type = "bob"
				})

				It("returns a CreateProcessMessage with only type, appGUID and spaceGUID set", func() {
					message := processInfo.ToProcessCreateMessage(appGUID, spaceGUID)

					Expect(message).To(Equal(repositories.CreateProcessMessage{
						Type:      "bob",
						AppGUID:   appGUID,
						SpaceGUID: spaceGUID,
					}))
				})
			})
		})

		Describe("ToProcessPatchMessage", func() {
			const processGUID = "the-process-guid"
			var processInfo ManifestApplicationProcess

			BeforeEach(func() {
				processInfo = ManifestApplicationProcess{Type: "web"}
			})

			Describe("HealthCheckType", func() {
				When("HealthCheckType is specified as 'none' (legacy alias for 'process')", func() {
					const noneHealthCheckType = "none"

					It("converts the type to 'process'", func() {
						processInfo.HealthCheckType = tools.PtrTo(noneHealthCheckType)

						message := processInfo.ToProcessPatchMessage(processGUID, spaceGUID)

						Expect(message.HealthCheckType).To(Equal(tools.PtrTo("process")))
					})
				})

				When("HealthCheckType is specified as some other valid type", func() {
					It("passes the type through to the message", func() {
						processInfo.HealthCheckType = tools.PtrTo("port")

						message := processInfo.ToProcessPatchMessage(processGUID, spaceGUID)

						Expect(message.HealthCheckType).To(Equal(tools.PtrTo("port")))
					})
				})

				When("HealthCheckType is unspecified", func() {
					It("returns a message with HealthCheckType unset", func() {
						Expect(
							processInfo.ToProcessPatchMessage(processGUID, spaceGUID).HealthCheckType,
						).To(BeNil())
					})
				})
			})

			When("DiskQuota is specified", func() {
				BeforeEach(func() {
					processInfo.DiskQuota = tools.PtrTo("1G")
				})

				It("returns a message with DiskQuotaMB set to the parsed value", func() {
					Expect(
						processInfo.ToProcessPatchMessage(processGUID, spaceGUID).DiskQuotaMB,
					).To(PointTo(BeEquivalentTo(1024)))
				})
			})

			When("DiskQuota is unspecified", func() {
				It("returns a message with DiskQuotaMB unset", func() {
					Expect(
						processInfo.ToProcessPatchMessage(processGUID, spaceGUID).DiskQuotaMB,
					).To(BeNil())
				})
			})

			When("Memory is specified", func() {
				BeforeEach(func() {
					processInfo.Memory = tools.PtrTo("1G")
				})

				It("returns a message with MemoryMB set to the parsed value", func() {
					Expect(
						processInfo.ToProcessPatchMessage(processGUID, spaceGUID).MemoryMB,
					).To(PointTo(BeEquivalentTo(1024)))
				})
			})

			When("Memory is unspecified", func() {
				It("returns a message with MemoryMB unset", func() {
					Expect(
						processInfo.ToProcessPatchMessage(processGUID, spaceGUID).MemoryMB,
					).To(BeNil())
				})
			})

			When("Instances is specified", func() {
				BeforeEach(func() {
					processInfo.Instances = tools.PtrTo(3)
				})

				It("returns a message with DesiredInstances set to the parsed value", func() {
					Expect(
						processInfo.ToProcessPatchMessage(processGUID, spaceGUID).DesiredInstances,
					).To(PointTo(BeEquivalentTo(3)))
				})
			})

			When("Instances is unspecified", func() {
				It("returns a message with DesiredInstances unset", func() {
					Expect(
						processInfo.ToProcessPatchMessage(processGUID, spaceGUID).DesiredInstances,
					).To(BeNil())
				})
			})
		})
	})

	Describe("ManifestRoute", func() {
		BeforeEach(func() {
			testSpaceManifest.Applications[0].Routes = []ManifestRoute{{}}
		})

		It("validates the struct", func() {
			Expect(validateErr).NotTo(HaveOccurred())
		})

		When("the route is not valid", func() {
			BeforeEach(func() {
				testSpaceManifest.Applications[0].Routes[0].Route = tools.PtrTo("httpp://invalidprotocol.net")
			})

			It("returns a validation error", func() {
				Expect(validateErr).To(MatchError(ContainSubstring("route: is not a valid route.")))
			})
		})
	})
})
