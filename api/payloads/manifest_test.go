package payloads_test

import (
	. "code.cloudfoundry.org/korifi/api/payloads"
	"code.cloudfoundry.org/korifi/api/repositories"
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/tools"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	"go.yaml.in/yaml/v3"
)

var _ = Describe("Manifest payload", func() {
	const spaceGUID = "the-space-guid"

	Describe("Manifest", func() {
		Describe("Validate", func() {
			var (
				testSpaceManifest Manifest
				validateErr       error
			)

			BeforeEach(func() {
				testSpaceManifest = Manifest{
					Applications: []ManifestApplication{{
						Name:         "test-app",
						DefaultRoute: true,
					}},
				}
			})

			JustBeforeEach(func() {
				validateErr = validator.DecodeAndValidateYAMLPayload(createYAMLRequest(testSpaceManifest), &Manifest{})
			})

			It("validates the struct", func() {
				Expect(validateErr).NotTo(HaveOccurred())
			})

			When("an application yaml is invalid", func() {
				BeforeEach(func() {
					testSpaceManifest.Applications[0].Memory = tools.PtrTo("badmemory")
				})

				It("returns a validation error", func() {
					expectUnprocessableEntityError(validateErr, "applications[0].memory must use a supported unit (B, K, KB, M, m, MB, mb, G, g, GB, gb, T, t, TB or tb)")
				})

				When("there is more than one error", func() {
					BeforeEach(func() {
						testSpaceManifest.Applications[0].DiskQuota = tools.PtrTo("baddisk")
					})

					It("returns both errors", func() {
						expectUnprocessableEntityError(validateErr, "applications[0].disk_quota must use a supported unit (B, K, KB, M, m, MB, mb, G, g, GB, gb, T, t, TB or tb)")
						expectUnprocessableEntityError(validateErr, "applications[0].memory must use a supported unit (B, K, KB, M, m, MB, mb, G, g, GB, gb, T, t, TB or tb)")
					})
				})
			})
		})
	})

	Describe("ManifestApplication", func() {
		var (
			testManifest ManifestApplication
			validateErr  error
		)

		Describe("Validate", func() {
			BeforeEach(func() {
				testManifest = ManifestApplication{
					Name:         "test-app",
					DefaultRoute: true,
				}
			})

			JustBeforeEach(func() {
				validateErr = validator.DecodeAndValidateYAMLPayload(createYAMLRequest(testManifest), &ManifestApplication{})
			})

			It("validates the struct", func() {
				Expect(validateErr).NotTo(HaveOccurred())
			})

			When("Name is empty", func() {
				BeforeEach(func() {
					testManifest.Name = ""
				})

				It("returns a validation error", func() {
					expectUnprocessableEntityError(validateErr, "name cannot be blank")
				})
			})

			When("Instances is negative", func() {
				BeforeEach(func() {
					testManifest.Instances = tools.PtrTo[int32](-1)
				})

				It("returns a validation error", func() {
					expectUnprocessableEntityError(validateErr, "instances must be no less than 0")
				})
			})

			When("the disk quota doesn't supply a unit", func() {
				BeforeEach(func() {
					testManifest.DiskQuota = tools.PtrTo("1024")
				})

				It("returns a validation error", func() {
					expectUnprocessableEntityError(validateErr, "disk_quota must use a supported unit (B, K, KB, M, m, MB, mb, G, g, GB, gb, T, t, TB or tb)")
				})
			})

			When("the disk quota is not positive", func() {
				BeforeEach(func() {
					testManifest.DiskQuota = tools.PtrTo("0MB")
				})

				It("returns a validation error", func() {
					expectUnprocessableEntityError(validateErr, "disk_quota must be greater than 0MB")
				})
			})

			When("the disk quota is a floating point number", func() {
				BeforeEach(func() {
					testManifest.DiskQuota = tools.PtrTo("1.5G")
				})

				It("does not return a validation error", func() {
					Expect(validateErr).NotTo(HaveOccurred())
				})
			})

			When("the alt disk quota doesn't supply a unit", func() {
				BeforeEach(func() {
					testManifest.AltDiskQuota = tools.PtrTo("1024")
				})

				It("returns a validation error", func() {
					expectUnprocessableEntityError(validateErr, "disk-quota must use a supported unit (B, K, KB, M, m, MB, mb, G, g, GB, gb, T, t, TB or tb)")
				})
			})

			When("the alt disk quota is not positive", func() {
				BeforeEach(func() {
					testManifest.AltDiskQuota = tools.PtrTo("0MB")
				})

				It("returns a validation error", func() {
					expectUnprocessableEntityError(validateErr, "disk-quota must be greater than 0MB")
				})
			})

			When("app disk-quota and app disk_quota are both set", func() {
				BeforeEach(func() {
					testManifest.DiskQuota = tools.PtrTo("128M")
					testManifest.AltDiskQuota = tools.PtrTo("128M")
				})

				It("response with an unprocessable entity error", func() {
					expectUnprocessableEntityError(validateErr, "disk_quota and disk-quota may not be used together")
				})
			})

			When("HealthCheckInvocationTimeout is not positive", func() {
				BeforeEach(func() {
					testManifest.HealthCheckInvocationTimeout = tools.PtrTo(int32(0))
				})

				It("returns a validation error", func() {
					expectUnprocessableEntityError(validateErr, "health-check-invocation-timeout must be no less than 1")
				})
			})

			When("HealthCheckType is invalid", func() {
				BeforeEach(func() {
					testManifest.HealthCheckType = tools.PtrTo("FakeHealthcheckType")
				})

				It("returns a validation error", func() {
					expectUnprocessableEntityError(validateErr, "health-check-type must be a valid value")
				})
			})

			When("Timeout is not positive", func() {
				BeforeEach(func() {
					testManifest.Timeout = tools.PtrTo(int32(0))
				})

				It("returns a validation error", func() {
					expectUnprocessableEntityError(validateErr, "timeout must be no less than 1")
				})
			})

			When("Memory units not valid", func() {
				BeforeEach(func() {
					testManifest.Memory = tools.PtrTo("5CUPS")
				})

				It("returns a validation error", func() {
					expectUnprocessableEntityError(validateErr, "memory must use a supported unit (B, K, KB, M, m, MB, mb, G, g, GB, gb, T, t, TB or tb)")
				})
			})

			When("the memory is not positive", func() {
				BeforeEach(func() {
					testManifest.Memory = tools.PtrTo("0MB")
				})

				It("returns a validation error", func() {
					expectUnprocessableEntityError(validateErr, "memory must be greater than 0MB")
				})
			})

			When("the memory is a floating point number", func() {
				BeforeEach(func() {
					testManifest.Memory = tools.PtrTo("0.5G")
				})

				It("does not return a validation error", func() {
					Expect(validateErr).NotTo(HaveOccurred())
				})
			})

			When("random-route and default-route flags are both set", func() {
				BeforeEach(func() {
					testManifest.DefaultRoute = true
					testManifest.RandomRoute = true
				})

				It("response with an unprocessable entity error", func() {
					expectUnprocessableEntityError(validateErr, "default-route and random-route may not be used together")
				})
			})

			When("only the random-route flag is set", func() {
				BeforeEach(func() {
					testManifest.DefaultRoute = false
					testManifest.RandomRoute = true
				})

				It("does not return a validation error", func() {
					Expect(validateErr).NotTo(HaveOccurred())
				})
			})

			When("buildpack is specified", func() {
				BeforeEach(func() {
					testManifest.Buildpack = tools.PtrTo("foo")
				})

				It("does not return a validation error", func() {
					Expect(validateErr).NotTo(HaveOccurred())
				})

				When("docker is specified", func() {
					BeforeEach(func() {
						testManifest.Docker = struct{}{}
					})
				})
			})

			When("buildpacks are specified", func() {
				BeforeEach(func() {
					testManifest.Buildpacks = []string{"foo"}
				})

				It("does not return a validation error", func() {
					Expect(validateErr).NotTo(HaveOccurred())
				})

				When("docker is specified", func() {
					BeforeEach(func() {
						testManifest.Docker = struct{}{}
					})
				})
			})

			When("docker is specified", func() {
				BeforeEach(func() {
					testManifest.Docker = struct{}{}
				})

				It("does not return a validation error", func() {
					Expect(validateErr).NotTo(HaveOccurred())
				})

				When("buildpack is specified", func() {
					BeforeEach(func() {
						testManifest.Buildpack = tools.PtrTo("foo")
					})

					It("response with an unprocessable entity error", func() {
						expectUnprocessableEntityError(validateErr, "docker must be blank when buildpacks are specified")
					})
				})

				When("buildpacks are specified", func() {
					BeforeEach(func() {
						testManifest.Buildpacks = []string{"foo"}
					})

					It("response with an unprocessable entity error", func() {
						expectUnprocessableEntityError(validateErr, "docker must be blank when buildpacks are specified")
					})
				})
			})
		})

		Describe("ToAppCreateMessage", func() {
			var createMessage repositories.CreateAppMessage

			JustBeforeEach(func() {
				createMessage = testManifest.ToAppCreateMessage(spaceGUID)
			})

			Describe("buildpack app", func() {
				BeforeEach(func() {
					testManifest = ManifestApplication{
						Name:       "my-app",
						Buildpacks: []string{"bp1"},
						Env: map[string]string{
							"e1": "env1",
						},
						Metadata: MetadataPatch{
							Annotations: map[string]*string{
								"a1": tools.PtrTo("v1"),
							},
							Labels: map[string]*string{
								"l2": tools.PtrTo("v2"),
							},
						},
					}
				})

				It("creates a buildpack app create message", func() {
					Expect(createMessage).To(Equal(repositories.CreateAppMessage{
						Name:      "my-app",
						SpaceGUID: spaceGUID,
						State:     repositories.DesiredState(korifiv1alpha1.StoppedState),
						Lifecycle: repositories.Lifecycle{
							Type: string(korifiv1alpha1.BuildpackLifecycle),
							Data: repositories.LifecycleData{
								Buildpacks: []string{"bp1"},
							},
						},
						EnvironmentVariables: map[string]string{
							"e1": "env1",
						},
						Metadata: repositories.Metadata{
							Annotations: map[string]string{
								"a1": "v1",
							},
							Labels: map[string]string{
								"l2": "v2",
							},
						},
					}))
				})
			})

			Describe("docker app", func() {
				BeforeEach(func() {
					testManifest = ManifestApplication{
						Name: "my-app",
						Env: map[string]string{
							"e1": "env1",
						},
						Metadata: MetadataPatch{
							Annotations: map[string]*string{
								"a1": tools.PtrTo("v1"),
							},
							Labels: map[string]*string{
								"l2": tools.PtrTo("v2"),
							},
						},
						Docker: struct{}{},
					}
				})

				It("creates a buildpack app create message", func() {
					Expect(createMessage).To(Equal(repositories.CreateAppMessage{
						Name:      "my-app",
						SpaceGUID: spaceGUID,
						State:     repositories.DesiredState(korifiv1alpha1.StoppedState),
						Lifecycle: repositories.Lifecycle{
							Type: string(korifiv1alpha1.DockerPackage),
						},
						EnvironmentVariables: map[string]string{
							"e1": "env1",
						},
						Metadata: repositories.Metadata{
							Annotations: map[string]string{
								"a1": "v1",
							},
							Labels: map[string]string{
								"l2": "v2",
							},
						},
					}))
				})
			})
		})
	})

	Describe("ManifestApplicationProcess", func() {
		Describe("Validate", func() {
			var (
				testManifestProcess ManifestApplicationProcess
				validateErr         error
			)

			BeforeEach(func() {
				testManifestProcess = ManifestApplicationProcess{
					Type: "some-type",
				}
			})

			JustBeforeEach(func() {
				validateErr = validator.DecodeAndValidateYAMLPayload(createYAMLRequest(testManifestProcess), &ManifestApplicationProcess{})
			})

			It("Validates the struct", func() {
				Expect(validateErr).NotTo(HaveOccurred())
			})

			When("the type is empty", func() {
				BeforeEach(func() {
					testManifestProcess.Type = ""
				})

				It("returns a validation error", func() {
					expectUnprocessableEntityError(validateErr, "type cannot be blank")
				})
			})

			When("the disk quota doesn't supply a unit", func() {
				BeforeEach(func() {
					testManifestProcess.DiskQuota = tools.PtrTo("1024")
				})

				It("returns a validation error", func() {
					expectUnprocessableEntityError(validateErr, "disk_quota must use a supported unit (B, K, KB, M, m, MB, mb, G, g, GB, gb, T, t, TB or tb)")
				})
			})

			When("the disk quota is not positive", func() {
				BeforeEach(func() {
					testManifestProcess.DiskQuota = tools.PtrTo("0MB")
				})

				It("returns a validation error", func() {
					expectUnprocessableEntityError(validateErr, "disk_quota must be greater than 0MB")
				})
			})

			When("the disk quota is a floating point number", func() {
				BeforeEach(func() {
					testManifestProcess.DiskQuota = tools.PtrTo("1.5G")
				})

				It("does not return a validation error", func() {
					Expect(validateErr).NotTo(HaveOccurred())
				})
			})

			When("the alt disk quota doesn't supply a unit", func() {
				BeforeEach(func() {
					testManifestProcess.AltDiskQuota = tools.PtrTo("1024")
				})

				It("returns a validation error", func() {
					expectUnprocessableEntityError(validateErr, "disk-quota must use a supported unit (B, K, KB, M, m, MB, mb, G, g, GB, gb, T, t, TB or tb)")
				})
			})

			When("the alt disk quota is not positive", func() {
				BeforeEach(func() {
					testManifestProcess.AltDiskQuota = tools.PtrTo("0MB")
				})

				It("returns a validation error", func() {
					expectUnprocessableEntityError(validateErr, "disk-quota must be greater than 0MB")
				})
			})

			When("app disk-quota and app disk_quota are both set", func() {
				BeforeEach(func() {
					testManifestProcess.DiskQuota = tools.PtrTo("128M")
					testManifestProcess.AltDiskQuota = tools.PtrTo("128M")
				})

				It("response with an unprocessable entity error", func() {
					expectUnprocessableEntityError(validateErr, "disk_quota and disk-quota may not be used together")
				})
			})

			When("HealthCheckInvocationTimeout is not positive", func() {
				BeforeEach(func() {
					testManifestProcess.HealthCheckInvocationTimeout = tools.PtrTo(int32(0))
				})

				It("returns a validation error", func() {
					expectUnprocessableEntityError(validateErr, "health-check-invocation-timeout must be no less than 1")
				})
			})

			When("HealthCheckType is invalid", func() {
				BeforeEach(func() {
					testManifestProcess.HealthCheckType = tools.PtrTo("FakeHealthcheckType")
				})

				It("returns a validation error", func() {
					expectUnprocessableEntityError(validateErr, "health-check-type must be a valid value")
				})
			})

			When("Instances is negative", func() {
				BeforeEach(func() {
					testManifestProcess.Instances = tools.PtrTo[int32](-1)
				})

				It("returns a validation error", func() {
					expectUnprocessableEntityError(validateErr, "instances must be no less than 0")
				})
			})

			When("the memory doesn't supply a unit", func() {
				BeforeEach(func() {
					testManifestProcess.Memory = tools.PtrTo("1024")
				})

				It("returns a validation error", func() {
					expectUnprocessableEntityError(validateErr, "memory must use a supported unit (B, K, KB, M, m, MB, mb, G, g, GB, gb, T, t, TB or tb)")
				})
			})

			When("the memory is not positive", func() {
				BeforeEach(func() {
					testManifestProcess.Memory = tools.PtrTo("0MB")
				})

				It("returns a validation error", func() {
					expectUnprocessableEntityError(validateErr, "memory must be greater than 0MB")
				})
			})

			When("the memory is a floating point number", func() {
				BeforeEach(func() {
					testManifestProcess.Memory = tools.PtrTo("0.5G")
				})

				It("does not return a validation error", func() {
					Expect(validateErr).NotTo(HaveOccurred())
				})
			})

			When("Timeout is not positive", func() {
				BeforeEach(func() {
					testManifestProcess.Timeout = tools.PtrTo(int32(0))
				})

				It("returns a validation error", func() {
					expectUnprocessableEntityError(validateErr, "timeout must be no less than 1")
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
						HealthCheckInvocationTimeout: tools.PtrTo(int32(90)),
						HealthCheckType:              tools.PtrTo("http"),
						Instances:                    tools.PtrTo[int32](3),
						Memory:                       tools.PtrTo("1G"),
						Timeout:                      tools.PtrTo(int32(60)),
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
						DesiredInstances: tools.PtrTo[int32](3),
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
					processInfo.Instances = tools.PtrTo[int32](3)
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
		var (
			validateErr       error
			testManifestRoute ManifestRoute
		)
		BeforeEach(func() {
			testManifestRoute = ManifestRoute{}
		})
		JustBeforeEach(func() {
			validateErr = validator.DecodeAndValidateYAMLPayload(createYAMLRequest(testManifestRoute), &ManifestRoute{})
		})
		It("validates the struct", func() {
			Expect(validateErr).NotTo(HaveOccurred())
		})

		When("the route is not valid", func() {
			BeforeEach(func() {
				testManifestRoute.Route = tools.PtrTo("httpp://invalidprotocol.net")
			})

			It("returns a validation error", func() {
				expectUnprocessableEntityError(validateErr, "route is not a valid route")
			})
		})
	})

	Describe("ManifestApplicationService", func() {
		Describe("Unmarshall", func() {
			var (
				unmarshalErr        error
				serviceString       string
				unmarshalledService ManifestApplicationService
			)

			BeforeEach(func() {
				serviceString = "name: my-svc"
				unmarshalledService = ManifestApplicationService{}
			})

			JustBeforeEach(func() {
				unmarshalErr = yaml.Unmarshal([]byte(serviceString), &unmarshalledService)
			})

			It("succeeds", func() {
				Expect(unmarshalErr).NotTo(HaveOccurred())
				Expect(unmarshalledService).To(Equal(ManifestApplicationService{
					Name: "my-svc",
				}))
			})

			When("the name tag is missing", func() {
				BeforeEach(func() {
					serviceString = "my-svc"
				})

				It("succeeds", func() {
					Expect(unmarshalErr).NotTo(HaveOccurred())
					Expect(unmarshalledService).To(Equal(ManifestApplicationService{
						Name: "my-svc",
					}))
				})

				When("the value is not a string", func() {
					BeforeEach(func() {
						serviceString = "123"
					})

					It("errors", func() {
						Expect(unmarshalErr).To(MatchError(ContainSubstring("invalid service")))
					})
				})
			})
		})

		Describe("Validate", func() {
			var (
				validateErr          error
				testManifestServices ManifestApplicationService
			)

			BeforeEach(func() {
				testManifestServices = ManifestApplicationService{
					Name: "my-service",
				}
			})

			JustBeforeEach(func() {
				validateErr = validator.DecodeAndValidateYAMLPayload(createYAMLRequest(testManifestServices), &ManifestApplicationService{})
			})

			It("validates the struct", func() {
				Expect(validateErr).NotTo(HaveOccurred())
			})

			When("name is not specified", func() {
				BeforeEach(func() {
					testManifestServices.Name = ""
				})

				It("returns a validation error", func() {
					expectUnprocessableEntityError(validateErr, "name cannot be blank")
				})
			})
		})
	})
})
