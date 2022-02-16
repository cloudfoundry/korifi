package repositories_test

import (
	"context"
	"time"

	"code.cloudfoundry.org/cf-k8s-controllers/api/repositories"
	workloadsv1alpha1 "code.cloudfoundry.org/cf-k8s-controllers/controllers/apis/workloads/v1alpha1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("ProcessRepo", func() {
	var (
		ctx                       context.Context
		processRepo               *repositories.ProcessRepo
		namespace1                *corev1.Namespace
		spaceDeveloperClusterRole *rbacv1.ClusterRole
		app1GUID                  string
		process1GUID              string
	)

	BeforeEach(func() {
		ctx = context.Background()
		processRepo = repositories.NewProcessRepo(k8sClient, userClientFactory)

		namespace1 = &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: prefixedGUID("namespace1")}}
		Expect(k8sClient.Create(ctx, namespace1)).To(Succeed())

		spaceDeveloperClusterRole = createClusterRole(ctx, repositories.SpaceDeveloperClusterRoleRules)

		app1GUID = prefixedGUID("app1")
		process1GUID = prefixedGUID("process1")
	})

	Describe("GetProcess", func() {
		var (
			cfProcess1     *workloadsv1alpha1.CFProcess
			getProcessGUID string
			processRecord  repositories.ProcessRecord
			getErr         error
		)

		BeforeEach(func() {
			cfProcess1 = createProcessCR(context.Background(), k8sClient, process1GUID, namespace1.Name, app1GUID)
			getProcessGUID = process1GUID
		})

		JustBeforeEach(func() {
			processRecord, getErr = processRepo.GetProcess(ctx, authInfo, getProcessGUID)
		})

		When("the user is not authorized in the space", func() {
			It("returns a forbidden error", func() {
				Expect(getErr).To(BeAssignableToTypeOf(repositories.ForbiddenError{}))
			})
		})

		When("the user has permission to get the process", func() {
			BeforeEach(func() {
				createClusterRoleBinding(ctx, userName, spaceDeveloperClusterRole.Name)
			})

			It("returns a Process record for the Process CR we request", func() {
				Expect(getErr).NotTo(HaveOccurred())

				Expect(processRecord.GUID).To(Equal(process1GUID))
				Expect(processRecord.SpaceGUID).To(Equal(namespace1.Name))
				Expect(processRecord.AppGUID).To(Equal(app1GUID))
				Expect(processRecord.Type).To(Equal(cfProcess1.Spec.ProcessType))
				Expect(processRecord.Command).To(Equal(cfProcess1.Spec.Command))
				Expect(processRecord.DesiredInstances).To(Equal(cfProcess1.Spec.DesiredInstances))
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
				Expect(getErr).To(MatchError(ContainSubstring("get-process: privileged client list failed")))
			})
		})

		When("duplicate Processes exist across namespaces with the same GUIDs", func() {
			var (
				namespace2 *corev1.Namespace
				app2GUID   string
			)

			When("duplicate Processes exist across namespaces with the same GUIDs", func() {
				BeforeEach(func() {
					app2GUID = prefixedGUID("app2")

					namespace2 = &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: prefixedGUID("namespace2")}}
					Expect(k8sClient.Create(context.Background(), namespace2)).To(Succeed())

					_ = createProcessCR(context.Background(), k8sClient, process1GUID, namespace2.Name, app2GUID)
				})

				It("returns an untyped error", func() {
					Expect(getErr).To(HaveOccurred())
					Expect(getErr).To(MatchError("duplicate processes exist"))
				})
			})

			When("no matching processes exist", func() {
				BeforeEach(func() {
					getProcessGUID = "i don't exist"
				})

				It("returns a not found error", func() {
					Expect(getErr).To(HaveOccurred())
					Expect(getErr).To(MatchError(repositories.NewNotFoundError("Process", nil)))
				})
			})

			It("returns a not found error when the user has no permission to see the process", func() {
				Expect(getErr).To(HaveOccurred())
				Expect(getErr).To(BeAssignableToTypeOf(repositories.ForbiddenError{}))
			})
		})
	})

	Describe("ListProcesses", func() {
		var (
			namespace2GUID string
			app2GUID       string
			process2GUID   string

			listProcessesMessage repositories.ListProcessesMessage
			processes            []repositories.ProcessRecord
		)

		BeforeEach(func() {
			namespace2GUID = prefixedGUID("namespace2")
			app2GUID = prefixedGUID("app2")
			process2GUID = prefixedGUID("process2")

			Expect(k8sClient.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace2GUID}})).To(Succeed())

			_ = createProcessCR(context.Background(), k8sClient, process1GUID, namespace1.Name, app1GUID)
			_ = createProcessCR(context.Background(), k8sClient, process2GUID, namespace2GUID, app1GUID)

			listProcessesMessage = repositories.ListProcessesMessage{
				AppGUID: []string{app1GUID},
			}
		})

		JustBeforeEach(func() {
			var err error
			processes, err = processRepo.ListProcesses(ctx, authInfo, listProcessesMessage)
			Expect(err).NotTo(HaveOccurred())
		})

		It("returns Process records for the AppGUID we request", func() {
			Expect(processes).To(ConsistOf(
				MatchFields(IgnoreExtras, Fields{"GUID": Equal(process1GUID)}),
				MatchFields(IgnoreExtras, Fields{"GUID": Equal(process2GUID)}),
			))
		})

		When("spaceGUID is supplied", func() {
			BeforeEach(func() {
				listProcessesMessage.SpaceGUID = namespace1.Name
			})

			It("returns the matching process in the given space", func() {
				Expect(processes).To(ConsistOf(
					MatchFields(IgnoreExtras, Fields{"GUID": Equal(process1GUID)}),
				))
			})
		})

		When("no Processes exist for an app", func() {
			BeforeEach(func() {
				listProcessesMessage.AppGUID = []string{app2GUID}
				listProcessesMessage.SpaceGUID = namespace1.Name
			})

			It("returns an empty list", func() {
				Expect(processes).To(BeEmpty())
				Expect(processes).ToNot(BeNil())
			})
		})
	})

	Describe("ScaleProcess", func() {
		var (
			cfProcess           *workloadsv1alpha1.CFProcess
			scaleProcessMessage *repositories.ScaleProcessMessage

			instanceScale int
			diskScaleMB   int64
			memoryScaleMB int64
		)

		BeforeEach(func() {
			cfProcess = createProcessCR(context.Background(), k8sClient, process1GUID, namespace1.Name, app1GUID)

			scaleProcessMessage = &repositories.ScaleProcessMessage{
				GUID:               process1GUID,
				SpaceGUID:          namespace1.Name,
				ProcessScaleValues: repositories.ProcessScaleValues{},
			}

			instanceScale = 7
			diskScaleMB = 80
			memoryScaleMB = 900
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
					Expect(scaleProcessRecord.DesiredInstances).To(Equal(cfProcess.Spec.DesiredInstances))
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

			var updatedCFProcess workloadsv1alpha1.CFProcess
			Expect(k8sClient.Get(
				ctx,
				client.ObjectKey{Name: process1GUID, Namespace: namespace1.Name},
				&updatedCFProcess,
			)).To(Succeed())

			Expect(updatedCFProcess.Spec.DesiredInstances).To(Equal(instanceScale))
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

	Describe("CreateProcess", func() {
		It("creates a CFProcess resource", func() {
			Expect(
				processRepo.CreateProcess(ctx, authInfo, repositories.CreateProcessMessage{
					AppGUID:     app1GUID,
					SpaceGUID:   namespace1.Name,
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
				}),
			).To(Succeed())

			var list workloadsv1alpha1.CFProcessList
			Expect(k8sClient.List(ctx, &list, client.InNamespace(namespace1.Name))).To(Succeed())
			Expect(list.Items).To(HaveLen(1))

			process := list.Items[0]
			Expect(process.Name).NotTo(BeEmpty())
			Expect(process.Spec).To(Equal(workloadsv1alpha1.CFProcessSpec{
				AppRef:      corev1.LocalObjectReference{Name: app1GUID},
				ProcessType: "web",
				Command:     "start-web",
				HealthCheck: workloadsv1alpha1.HealthCheck{
					Type: "http",
					Data: workloadsv1alpha1.HealthCheckData{
						HTTPEndpoint:             "/healthz",
						InvocationTimeoutSeconds: 20,
						TimeoutSeconds:           10,
					},
				},
				DesiredInstances: 42,
				MemoryMB:         456,
				DiskQuotaMB:      123,
				Ports:            []int32{},
			}))
		})
	})

	Describe("GetProcessByAppTypeAndSpace", func() {
		const (
			processType = "thingy"
		)

		When("there is a matching process", func() {
			BeforeEach(func() {
				cfProcess := &workloadsv1alpha1.CFProcess{
					ObjectMeta: metav1.ObjectMeta{
						Name:      process1GUID,
						Namespace: namespace1.Name,
						Labels: map[string]string{
							cfAppGUIDLabelKey: app1GUID,
						},
					},
					Spec: workloadsv1alpha1.CFProcessSpec{
						AppRef: corev1.LocalObjectReference{
							Name: app1GUID,
						},
						ProcessType: processType,
						Command:     "the-command",
						HealthCheck: workloadsv1alpha1.HealthCheck{
							Type: "http",
							Data: workloadsv1alpha1.HealthCheckData{
								HTTPEndpoint:             "/healthz",
								InvocationTimeoutSeconds: 1,
								TimeoutSeconds:           2,
							},
						},
						DesiredInstances: 1,
						MemoryMB:         2,
						DiskQuotaMB:      3,
						Ports:            []int32{8080},
					},
				}

				Expect(k8sClient.Create(context.Background(), cfProcess)).To(Succeed())
			})

			It("returns it", func() {
				processRecord, err := processRepo.GetProcessByAppTypeAndSpace(ctx, authInfo, app1GUID, processType, namespace1.Name)
				Expect(err).NotTo(HaveOccurred())
				Expect(processRecord).To(MatchAllFields(Fields{
					"GUID":             Equal(process1GUID),
					"SpaceGUID":        Equal(namespace1.Name),
					"AppGUID":          Equal(app1GUID),
					"Type":             Equal(processType),
					"Command":          Equal("the-command"),
					"DesiredInstances": Equal(1),
					"MemoryMB":         BeEquivalentTo(2),
					"DiskQuotaMB":      BeEquivalentTo(3),
					"Ports":            Equal([]int32{8080}),
					"HealthCheck": Equal(repositories.HealthCheck{
						Type: "http",
						Data: repositories.HealthCheckData{
							HTTPEndpoint:             "/healthz",
							InvocationTimeoutSeconds: 1,
							TimeoutSeconds:           2,
						},
					}),
					"Labels":      BeEmpty(),
					"Annotations": BeEmpty(),
					"CreatedAt":   Not(BeEmpty()),
					"UpdatedAt":   Not(BeEmpty()),
				}))
			})
		})

		When("there is no matching process", func() {
			It("returns a NotFoundError", func() {
				_, err := processRepo.GetProcessByAppTypeAndSpace(ctx, authInfo, app1GUID, processType, namespace1.Name)
				Expect(err).To(MatchError(repositories.NewNotFoundError("Process", nil)))
			})
		})
	})

	Describe("PatchProcess", func() {
		When("the app already has a process with the given type", func() {
			var (
				cfProcess *workloadsv1alpha1.CFProcess
				message   repositories.PatchProcessMessage
			)

			BeforeEach(func() {
				cfProcess = &workloadsv1alpha1.CFProcess{
					ObjectMeta: metav1.ObjectMeta{
						Name:      process1GUID,
						Namespace: namespace1.Name,
						Labels: map[string]string{
							cfAppGUIDLabelKey: app1GUID,
						},
					},
					Spec: workloadsv1alpha1.CFProcessSpec{
						AppRef: corev1.LocalObjectReference{
							Name: app1GUID,
						},
						ProcessType: "web",
						Command:     "original-command",
						HealthCheck: workloadsv1alpha1.HealthCheck{
							Type: "process",
							Data: workloadsv1alpha1.HealthCheckData{
								InvocationTimeoutSeconds: 1,
								TimeoutSeconds:           2,
							},
						},
						DesiredInstances: 1,
						MemoryMB:         2,
						DiskQuotaMB:      3,
						Ports:            []int32{8080},
					},
				}

				Expect(k8sClient.Create(context.Background(), cfProcess)).To(Succeed())
			})

			When("all fields are set", func() {
				BeforeEach(func() {
					message = repositories.PatchProcessMessage{
						ProcessGUID:                         process1GUID,
						SpaceGUID:                           namespace1.Name,
						Command:                             stringPointer("start-web"),
						HealthCheckType:                     stringPointer("http"),
						HealthCheckHTTPEndpoint:             stringPointer("/healthz"),
						HealthCheckInvocationTimeoutSeconds: int64Pointer(20),
						HealthCheckTimeoutSeconds:           int64Pointer(10),
						DesiredInstances:                    intPointer(42),
						MemoryMB:                            int64Pointer(456),
						DiskQuotaMB:                         int64Pointer(123),
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

					var process workloadsv1alpha1.CFProcess
					Eventually(func() workloadsv1alpha1.CFProcessSpec {
						Expect(
							k8sClient.Get(ctx, types.NamespacedName{Name: process1GUID, Namespace: namespace1.Name}, &process),
						).To(Succeed())
						return process.Spec
					}).Should(Equal(workloadsv1alpha1.CFProcessSpec{
						AppRef:      corev1.LocalObjectReference{Name: app1GUID},
						ProcessType: "web",
						Command:     "start-web",
						HealthCheck: workloadsv1alpha1.HealthCheck{
							Type: "http",
							Data: workloadsv1alpha1.HealthCheckData{
								HTTPEndpoint:             "/healthz",
								InvocationTimeoutSeconds: 20,
								TimeoutSeconds:           10,
							},
						},
						DesiredInstances: 42,
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
						SpaceGUID:                 namespace1.Name,
						Command:                   stringPointer("new-command"),
						HealthCheckTimeoutSeconds: int64Pointer(42),
						DesiredInstances:          intPointer(5),
						MemoryMB:                  int64Pointer(123),
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

					var process workloadsv1alpha1.CFProcess
					Eventually(func() workloadsv1alpha1.CFProcessSpec {
						Expect(
							k8sClient.Get(ctx, types.NamespacedName{Name: process1GUID, Namespace: namespace1.Name}, &process),
						).To(Succeed())
						return process.Spec
					}).Should(Equal(workloadsv1alpha1.CFProcessSpec{
						AppRef:      corev1.LocalObjectReference{Name: app1GUID},
						ProcessType: "web",
						Command:     "new-command",
						HealthCheck: workloadsv1alpha1.HealthCheck{
							Type: "process",
							Data: workloadsv1alpha1.HealthCheckData{
								InvocationTimeoutSeconds: 1,
								TimeoutSeconds:           42,
							},
						},
						DesiredInstances: 5,
						MemoryMB:         123,
						DiskQuotaMB:      3,
						Ports:            []int32{8080},
					}))
				})
			})
		})
	})
})

func stringPointer(s string) *string {
	return &s
}

func intPointer(i int) *int {
	return &i
}

func int64Pointer(i int64) *int64 {
	return &i
}
