package repositories_test

import (
	"context"
	"time"

	"code.cloudfoundry.org/korifi/api/apierrors"
	"code.cloudfoundry.org/korifi/api/repositories"
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/tests/matchers"
	"code.cloudfoundry.org/korifi/tools"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gstruct"
	. "github.com/onsi/gomega/gstruct"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("ProcessRepo", func() {
	var (
		processRepo  *repositories.ProcessRepo
		org          *korifiv1alpha1.CFOrg
		space        *korifiv1alpha1.CFSpace
		app1GUID     string
		process1GUID string
	)

	BeforeEach(func() {
		processRepo = repositories.NewProcessRepo(namespaceRetriever, userClientFactory, nsPerms)
		org = createOrgWithCleanup(ctx, prefixedGUID("org"))
		space = createSpaceWithCleanup(ctx, org.Name, prefixedGUID("space"))
		app1GUID = prefixedGUID("app1")
		process1GUID = prefixedGUID("process1")
	})

	Describe("GetProcess", func() {
		var (
			cfProcess1     *korifiv1alpha1.CFProcess
			getProcessGUID string
			processRecord  repositories.ProcessRecord
			getErr         error
		)

		BeforeEach(func() {
			cfProcess1 = createProcessCR(context.Background(), k8sClient, process1GUID, space.Name, app1GUID)
			getProcessGUID = process1GUID
		})

		JustBeforeEach(func() {
			processRecord, getErr = processRepo.GetProcess(ctx, authInfo, getProcessGUID)
		})

		When("the user is not authorized in the space", func() {
			It("returns a forbidden error", func() {
				Expect(getErr).To(matchers.WrapErrorAssignableToTypeOf(apierrors.ForbiddenError{}))
			})
		})

		When("the user has permission to get the process", func() {
			BeforeEach(func() {
				createRoleBinding(ctx, userName, spaceDeveloperRole.Name, space.Name)
			})

			It("returns a Process record for the Process CR we request", func() {
				Expect(getErr).NotTo(HaveOccurred())

				Expect(processRecord.GUID).To(Equal(process1GUID))
				Expect(processRecord.SpaceGUID).To(Equal(space.Name))
				Expect(processRecord.AppGUID).To(Equal(app1GUID))
				Expect(processRecord.Type).To(Equal(cfProcess1.Spec.ProcessType))
				Expect(processRecord.Command).To(Equal(cfProcess1.Spec.Command))
				Expect(processRecord.DesiredInstances).To(Equal(*cfProcess1.Spec.DesiredInstances))
				Expect(processRecord.MemoryMB).To(Equal(cfProcess1.Spec.MemoryMB))
				Expect(processRecord.DiskQuotaMB).To(Equal(cfProcess1.Spec.DiskQuotaMB))
				Expect(processRecord.Ports).To(Equal(cfProcess1.Spec.Ports))
				Expect(processRecord.HealthCheck.Type).To(Equal(string(cfProcess1.Spec.HealthCheck.Type)))
				Expect(processRecord.HealthCheck.Data.InvocationTimeoutSeconds).To(Equal(cfProcess1.Spec.HealthCheck.Data.InvocationTimeoutSeconds))
				Expect(processRecord.HealthCheck.Data.TimeoutSeconds).To(Equal(cfProcess1.Spec.HealthCheck.Data.TimeoutSeconds))
				Expect(processRecord.HealthCheck.Data.HTTPEndpoint).To(Equal(cfProcess1.Spec.HealthCheck.Data.HTTPEndpoint))
			})
		})

		When("the privileged list call fails", func() {
			var cancelFn context.CancelFunc

			BeforeEach(func() {
				ctx, cancelFn = context.WithDeadline(ctx, time.Now().Add(-1*time.Minute))
			})

			AfterEach(func() {
				cancelFn()
			})

			It("returns an untyped error", func() {
				Expect(getErr).To(HaveOccurred())
				Expect(getErr).To(MatchError(ContainSubstring("failed to list Process")))
			})
		})

		When("duplicate Processes exist across namespaces with the same GUIDs", func() {
			BeforeEach(func() {
				app2GUID := prefixedGUID("app2")

				namespace2 := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: prefixedGUID("namespace2")}}
				Expect(k8sClient.Create(context.Background(), namespace2)).To(Succeed())

				_ = createProcessCR(context.Background(), k8sClient, process1GUID, namespace2.Name, app2GUID)
			})

			It("returns an untyped error", func() {
				Expect(getErr).To(HaveOccurred())
				Expect(getErr).To(MatchError("get-process duplicate records exist"))
			})
		})

		When("no matching processes exist", func() {
			BeforeEach(func() {
				getProcessGUID = "i don't exist"
			})

			It("returns a not found error", func() {
				Expect(getErr).To(HaveOccurred())
				Expect(getErr).To(BeAssignableToTypeOf(apierrors.NotFoundError{}))
			})
		})
	})

	Describe("ListProcesses", func() {
		var (
			app2GUID       string
			process2GUID   string
			space1, space2 *korifiv1alpha1.CFSpace

			listProcessesMessage repositories.ListProcessesMessage
			processes            []repositories.ProcessRecord
		)

		BeforeEach(func() {
			space1 = createSpaceWithCleanup(ctx, org.Name, prefixedGUID("space1"))
			space2 = createSpaceWithCleanup(ctx, org.Name, prefixedGUID("space2"))

			app2GUID = prefixedGUID("app2")
			process2GUID = prefixedGUID("process2")

			_ = createProcessCR(context.Background(), k8sClient, process1GUID, space1.Name, app1GUID)
			_ = createProcessCR(context.Background(), k8sClient, process2GUID, space2.Name, app1GUID)

			listProcessesMessage = repositories.ListProcessesMessage{
				AppGUIDs: []string{app1GUID},
			}
		})

		JustBeforeEach(func() {
			var err error
			processes, err = processRepo.ListProcesses(ctx, authInfo, listProcessesMessage)
			Expect(err).NotTo(HaveOccurred())
		})

		It("returns an empty list to unauthorized users", func() {
			Expect(processes).To(BeEmpty())
		})

		When("the user is a space developer", func() {
			BeforeEach(func() {
				createRoleBinding(ctx, userName, spaceDeveloperRole.Name, space1.Name)
				createRoleBinding(ctx, userName, spaceDeveloperRole.Name, space2.Name)
			})

			It("returns Process records for the AppGUID we request", func() {
				Expect(processes).To(ConsistOf(
					MatchFields(IgnoreExtras, Fields{"GUID": Equal(process1GUID)}),
					MatchFields(IgnoreExtras, Fields{"GUID": Equal(process2GUID)}),
				))
			})

			When("spaceGUID is supplied", func() {
				BeforeEach(func() {
					listProcessesMessage.SpaceGUID = space1.Name
				})

				It("returns the matching process in the given space", func() {
					Expect(processes).To(ConsistOf(
						MatchFields(IgnoreExtras, Fields{"GUID": Equal(process1GUID)}),
					))
				})
			})

			When("no Processes exist for an app", func() {
				BeforeEach(func() {
					listProcessesMessage.AppGUIDs = []string{app2GUID}
					listProcessesMessage.SpaceGUID = space1.Name
				})

				It("returns an empty list", func() {
					Expect(processes).To(BeEmpty())
					Expect(processes).ToNot(BeNil())
				})
			})

			When("a space exists with a rolebinding for the user, but without permission to list processes", func() {
				BeforeEach(func() {
					anotherSpace := createSpaceWithCleanup(ctx, org.Name, "space-without-process-space-perm")
					createRoleBinding(ctx, userName, rootNamespaceUserRole.Name, anotherSpace.Name)
				})

				It("returns the processes", func() {
					Expect(processes).To(HaveLen(2))
				})
			})
		})
	})

	Describe("ScaleProcess", func() {
		var (
			space1              *korifiv1alpha1.CFSpace
			cfProcess           *korifiv1alpha1.CFProcess
			scaleProcessMessage *repositories.ScaleProcessMessage

			instanceScale int
			diskScaleMB   int64
			memoryScaleMB int64
		)

		BeforeEach(func() {
			space1 = createSpaceWithCleanup(ctx, org.Name, prefixedGUID("space1"))
			cfProcess = createProcessCR(context.Background(), k8sClient, process1GUID, space1.Name, app1GUID)

			scaleProcessMessage = &repositories.ScaleProcessMessage{
				GUID:               process1GUID,
				SpaceGUID:          space1.Name,
				ProcessScaleValues: repositories.ProcessScaleValues{},
			}

			instanceScale = 7
			diskScaleMB = 80
			memoryScaleMB = 900
		})

		It("returns a forbidden error to unauthorized users", func() {
			_, err := processRepo.ScaleProcess(ctx, authInfo, *scaleProcessMessage)
			Expect(err).To(matchers.WrapErrorAssignableToTypeOf(apierrors.ForbiddenError{}))
		})

		When("the user has the SpaceDeveloper role", func() {
			BeforeEach(func() {
				createRoleBinding(ctx, userName, spaceDeveloperRole.Name, space1.Name)
			})

			DescribeTable("calling ScaleProcess with a set of scale values returns an updated CFProcess record",
				func(instances *int, diskMB, memoryMB *int64) {
					scaleProcessMessage.ProcessScaleValues = repositories.ProcessScaleValues{
						Instances: instances,
						DiskMB:    diskMB,
						MemoryMB:  memoryMB,
					}
					scaleProcessRecord, scaleProcessErr := processRepo.ScaleProcess(context.Background(), authInfo, *scaleProcessMessage)
					Expect(scaleProcessErr).ToNot(HaveOccurred())
					if instances != nil {
						Expect(scaleProcessRecord.DesiredInstances).To(Equal(*instances))
					} else {
						Expect(scaleProcessRecord.DesiredInstances).To(Equal(*cfProcess.Spec.DesiredInstances))
					}
					if diskMB != nil {
						Expect(scaleProcessRecord.DiskQuotaMB).To(Equal(*diskMB))
					} else {
						Expect(scaleProcessRecord.DiskQuotaMB).To(Equal(cfProcess.Spec.DiskQuotaMB))
					}
					if memoryMB != nil {
						Expect(scaleProcessRecord.MemoryMB).To(Equal(*memoryMB))
					} else {
						Expect(scaleProcessRecord.MemoryMB).To(Equal(cfProcess.Spec.MemoryMB))
					}
				},

				Entry("all scale values are provided", &instanceScale, &diskScaleMB, &memoryScaleMB),
				Entry("no scale values are provided", nil, nil, nil),
				Entry("some scale values are provided", &instanceScale, nil, nil),
				Entry("some scale values are provided", nil, &diskScaleMB, &memoryScaleMB),
			)

			It("updates the scale of the CFProcess CR", func() {
				scaleProcessMessage.ProcessScaleValues = repositories.ProcessScaleValues{
					Instances: &instanceScale,
					MemoryMB:  &memoryScaleMB,
					DiskMB:    &diskScaleMB,
				}
				_, err := processRepo.ScaleProcess(ctx, authInfo, *scaleProcessMessage)
				Expect(err).ToNot(HaveOccurred())

				var updatedCFProcess korifiv1alpha1.CFProcess
				Expect(k8sClient.Get(
					ctx,
					client.ObjectKey{Name: process1GUID, Namespace: space1.Name},
					&updatedCFProcess,
				)).To(Succeed())

				Expect(updatedCFProcess.Spec.DesiredInstances).To(gstruct.PointTo(Equal(instanceScale)))
				Expect(updatedCFProcess.Spec.DiskQuotaMB).To(Equal(diskScaleMB))
				Expect(updatedCFProcess.Spec.MemoryMB).To(Equal(memoryScaleMB))
			})

			When("the process does not exist", func() {
				It("returns an error", func() {
					scaleProcessMessage.GUID = "i-dont-exist"
					_, err := processRepo.ScaleProcess(ctx, authInfo, *scaleProcessMessage)
					Expect(err).To(HaveOccurred())
				})
			})
		})
	})

	Describe("CreateProcess", func() {
		var createErr error
		JustBeforeEach(func() {
			createErr = processRepo.CreateProcess(ctx, authInfo, repositories.CreateProcessMessage{
				AppGUID:     app1GUID,
				SpaceGUID:   space.Name,
				Type:        "web",
				Command:     "start-web",
				DiskQuotaMB: 123,
				HealthCheck: repositories.HealthCheck{
					Type: "http",
					Data: repositories.HealthCheckData{
						HTTPEndpoint:             "/healthz",
						InvocationTimeoutSeconds: 20,
						TimeoutSeconds:           10,
					},
				},
				DesiredInstances: 42,
				MemoryMB:         456,
			})
		})

		When("user has permissions", func() {
			BeforeEach(func() {
				createRoleBinding(ctx, userName, spaceDeveloperRole.Name, space.Name)
			})

			It("creates a CFProcess resource", func() {
				Expect(createErr).NotTo(HaveOccurred())
				var list korifiv1alpha1.CFProcessList
				Expect(k8sClient.List(ctx, &list, client.InNamespace(space.Name))).To(Succeed())
				Expect(list.Items).To(HaveLen(1))

				process := list.Items[0]
				Expect(process.Name).To(HavePrefix("cf-proc-"))
				Expect(process.Name).To(HaveSuffix("-web"))
				Expect(process.Spec).To(Equal(korifiv1alpha1.CFProcessSpec{
					AppRef:      corev1.LocalObjectReference{Name: app1GUID},
					ProcessType: "web",
					Command:     "start-web",
					HealthCheck: korifiv1alpha1.HealthCheck{
						Type: "http",
						Data: korifiv1alpha1.HealthCheckData{
							HTTPEndpoint:             "/healthz",
							InvocationTimeoutSeconds: 20,
							TimeoutSeconds:           10,
						},
					},
					DesiredInstances: tools.PtrTo(42),
					MemoryMB:         456,
					DiskQuotaMB:      123,
					Ports:            []int32{},
				}))
			})
		})

		When("the user is not authorized in the space", func() {
			It("returns a forbidden error", func() {
				Expect(createErr).To(matchers.WrapErrorAssignableToTypeOf(apierrors.ForbiddenError{}))
			})
		})
	})

	Describe("GetProcessByAppTypeAndSpace", func() {
		const (
			processType = "web"
		)

		var (
			processRecord repositories.ProcessRecord
			getErr        error
		)

		JustBeforeEach(func() {
			processRecord, getErr = processRepo.GetProcessByAppTypeAndSpace(ctx, authInfo, app1GUID, processType, space.Name)
		})

		When("the user is not authorized in the space", func() {
			It("returns a forbidden error", func() {
				Expect(getErr).To(matchers.WrapErrorAssignableToTypeOf(apierrors.ForbiddenError{}))
			})
		})

		When("the user has permission to get the process", func() {
			BeforeEach(func() {
				createProcessCR(context.Background(), k8sClient, process1GUID, space.Name, app1GUID)
				createRoleBinding(ctx, userName, spaceDeveloperRole.Name, space.Name)
			})

			It("returns a Process record with the specified app type and space", func() {
				Expect(getErr).NotTo(HaveOccurred())
				Expect(processRecord).To(MatchAllFields(Fields{
					"GUID":             Equal(process1GUID),
					"SpaceGUID":        Equal(space.Name),
					"AppGUID":          Equal(app1GUID),
					"Type":             Equal(processType),
					"Command":          Equal(""),
					"DesiredInstances": Equal(1),
					"MemoryMB":         BeEquivalentTo(500),
					"DiskQuotaMB":      BeEquivalentTo(512),
					"Ports":            Equal([]int32{8080}),
					"HealthCheck": Equal(repositories.HealthCheck{
						Type: "process",
						Data: repositories.HealthCheckData{
							InvocationTimeoutSeconds: 0,
							TimeoutSeconds:           0,
						},
					}),
					"Labels":      BeEmpty(),
					"Annotations": BeEmpty(),
					"CreatedAt":   Not(BeEmpty()),
					"UpdatedAt":   Not(BeEmpty()),
				}))
			})

			When("there is no matching process", func() {
				BeforeEach(func() {
					app1GUID = "i don't exist"
				})

				It("returns a not found error", func() {
					Expect(getErr).To(MatchError(apierrors.NewNotFoundError(nil, repositories.ProcessResourceType)))
				})
			})
		})
	})

	Describe("PatchProcess", func() {
		When("the app already has a process with the given type", func() {
			var (
				cfProcess *korifiv1alpha1.CFProcess
				message   repositories.PatchProcessMessage
			)

			BeforeEach(func() {
				cfProcess = &korifiv1alpha1.CFProcess{
					ObjectMeta: metav1.ObjectMeta{
						Name:      process1GUID,
						Namespace: space.Name,
						Labels: map[string]string{
							cfAppGUIDLabelKey: app1GUID,
						},
					},
					Spec: korifiv1alpha1.CFProcessSpec{
						AppRef: corev1.LocalObjectReference{
							Name: app1GUID,
						},
						ProcessType: "web",
						Command:     "original-command",
						HealthCheck: korifiv1alpha1.HealthCheck{
							Type: "process",
							Data: korifiv1alpha1.HealthCheckData{
								InvocationTimeoutSeconds: 1,
								TimeoutSeconds:           2,
							},
						},
						DesiredInstances: tools.PtrTo(1),
						MemoryMB:         2,
						DiskQuotaMB:      3,
						Ports:            []int32{8080},
					},
				}

				Expect(k8sClient.Create(context.Background(), cfProcess)).To(Succeed())
			})

			When("users does not have permissions", func() {
				BeforeEach(func() {
					message = repositories.PatchProcessMessage{
						ProcessGUID:                         process1GUID,
						SpaceGUID:                           space.Name,
						Command:                             tools.PtrTo("start-web"),
						HealthCheckType:                     tools.PtrTo("http"),
						HealthCheckHTTPEndpoint:             tools.PtrTo("/healthz"),
						HealthCheckInvocationTimeoutSeconds: tools.PtrTo(int64(20)),
						HealthCheckTimeoutSeconds:           tools.PtrTo(int64(10)),
						DesiredInstances:                    tools.PtrTo(42),
						MemoryMB:                            tools.PtrTo(int64(456)),
						DiskQuotaMB:                         tools.PtrTo(int64(123)),
					}
				})

				It("returns a forbidden error to unauthorized users", func() {
					_, err := processRepo.PatchProcess(ctx, authInfo, message)
					Expect(err).To(matchers.WrapErrorAssignableToTypeOf(apierrors.ForbiddenError{}))
				})
			})

			When("user has permission", func() {
				BeforeEach(func() {
					createRoleBinding(ctx, userName, spaceDeveloperRole.Name, space.Name)
				})

				When("all fields are set", func() {
					BeforeEach(func() {
						message = repositories.PatchProcessMessage{
							ProcessGUID:                         process1GUID,
							SpaceGUID:                           space.Name,
							Command:                             tools.PtrTo("start-web"),
							HealthCheckType:                     tools.PtrTo("http"),
							HealthCheckHTTPEndpoint:             tools.PtrTo("/healthz"),
							HealthCheckInvocationTimeoutSeconds: tools.PtrTo(int64(20)),
							HealthCheckTimeoutSeconds:           tools.PtrTo(int64(10)),
							DesiredInstances:                    tools.PtrTo(42),
							MemoryMB:                            tools.PtrTo(int64(456)),
							DiskQuotaMB:                         tools.PtrTo(int64(123)),
						}
					})

					It("updates all fields on the existing CFProcess resource", func() {
						updatedProcessRecord, err := processRepo.PatchProcess(ctx, authInfo, message)
						Expect(err).NotTo(HaveOccurred())
						Expect(updatedProcessRecord.GUID).To(Equal(cfProcess.Name))
						Expect(updatedProcessRecord.SpaceGUID).To(Equal(cfProcess.Namespace))
						Expect(updatedProcessRecord.Command).To(Equal(*message.Command))
						Expect(updatedProcessRecord.HealthCheck.Type).To(Equal(*message.HealthCheckType))
						Expect(updatedProcessRecord.HealthCheck.Data.HTTPEndpoint).To(Equal(*message.HealthCheckHTTPEndpoint))
						Expect(updatedProcessRecord.HealthCheck.Data.TimeoutSeconds).To(Equal(*message.HealthCheckTimeoutSeconds))
						Expect(updatedProcessRecord.HealthCheck.Data.InvocationTimeoutSeconds).To(Equal(*message.HealthCheckInvocationTimeoutSeconds))
						Expect(updatedProcessRecord.DesiredInstances).To(Equal(*message.DesiredInstances))
						Expect(updatedProcessRecord.MemoryMB).To(Equal(*message.MemoryMB))
						Expect(updatedProcessRecord.DiskQuotaMB).To(Equal(*message.DiskQuotaMB))

						var process korifiv1alpha1.CFProcess
						Expect(k8sClient.Get(ctx, types.NamespacedName{Name: process1GUID, Namespace: space.Name}, &process)).To(Succeed())
						Expect(process.Spec).To(Equal(korifiv1alpha1.CFProcessSpec{
							AppRef:      corev1.LocalObjectReference{Name: app1GUID},
							ProcessType: "web",
							Command:     "start-web",
							HealthCheck: korifiv1alpha1.HealthCheck{
								Type: "http",
								Data: korifiv1alpha1.HealthCheckData{
									HTTPEndpoint:             "/healthz",
									InvocationTimeoutSeconds: 20,
									TimeoutSeconds:           10,
								},
							},
							DesiredInstances: tools.PtrTo(42),
							MemoryMB:         456,
							DiskQuotaMB:      123,
							Ports:            []int32{8080},
						}))
					})
				})

				When("only some fields are set", func() {
					BeforeEach(func() {
						message = repositories.PatchProcessMessage{
							ProcessGUID:               process1GUID,
							SpaceGUID:                 space.Name,
							Command:                   tools.PtrTo("new-command"),
							HealthCheckTimeoutSeconds: tools.PtrTo(int64(42)),
							DesiredInstances:          tools.PtrTo(5),
							MemoryMB:                  tools.PtrTo(int64(123)),
						}
					})

					It("patches only the provided fields on the Process", func() {
						updatedProcessRecord, err := processRepo.PatchProcess(ctx, authInfo, message)
						Expect(err).NotTo(HaveOccurred())
						Expect(updatedProcessRecord.GUID).To(Equal(cfProcess.Name))
						Expect(updatedProcessRecord.SpaceGUID).To(Equal(cfProcess.Namespace))
						Expect(updatedProcessRecord.Command).To(Equal(*message.Command))
						Expect(updatedProcessRecord.HealthCheck.Type).To(Equal(string(cfProcess.Spec.HealthCheck.Type)))
						Expect(updatedProcessRecord.HealthCheck.Data.HTTPEndpoint).To(Equal(cfProcess.Spec.HealthCheck.Data.HTTPEndpoint))
						Expect(updatedProcessRecord.HealthCheck.Data.TimeoutSeconds).To(Equal(*message.HealthCheckTimeoutSeconds))
						Expect(updatedProcessRecord.HealthCheck.Data.InvocationTimeoutSeconds).To(Equal(cfProcess.Spec.HealthCheck.Data.InvocationTimeoutSeconds))
						Expect(updatedProcessRecord.DesiredInstances).To(Equal(*message.DesiredInstances))
						Expect(updatedProcessRecord.MemoryMB).To(Equal(*message.MemoryMB))
						Expect(updatedProcessRecord.DiskQuotaMB).To(Equal(cfProcess.Spec.DiskQuotaMB))

						var process korifiv1alpha1.CFProcess
						Expect(k8sClient.Get(ctx, types.NamespacedName{Name: process1GUID, Namespace: space.Name}, &process)).To(Succeed())

						Expect(process.Spec).To(Equal(korifiv1alpha1.CFProcessSpec{
							AppRef:      corev1.LocalObjectReference{Name: app1GUID},
							ProcessType: "web",
							Command:     "new-command",
							HealthCheck: korifiv1alpha1.HealthCheck{
								Type: "process",
								Data: korifiv1alpha1.HealthCheckData{
									InvocationTimeoutSeconds: 1,
									TimeoutSeconds:           42,
								},
							},
							DesiredInstances: tools.PtrTo(5),
							MemoryMB:         123,
							DiskQuotaMB:      3,
							Ports:            []int32{8080},
						}))
					})
				})
			})
		})
	})
})
