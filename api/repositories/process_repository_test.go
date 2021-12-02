package repositories_test

import (
	"context"
	"time"

	. "github.com/onsi/gomega/gstruct"

	. "code.cloudfoundry.org/cf-k8s-controllers/api/repositories"
	workloadsv1alpha1 "code.cloudfoundry.org/cf-k8s-controllers/controllers/apis/workloads/v1alpha1"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("ProcessRepo", func() {
	var (
		testCtx      context.Context
		processRepo  *ProcessRepo
		namespace    *corev1.Namespace
		app1GUID     string
		app2GUID     string
		process1GUID string
		process2GUID string
	)

	BeforeEach(func() {
		testCtx = context.Background()

		processRepo = NewProcessRepo(k8sClient)

		namespaceName := generateGUID()
		namespace = &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespaceName}}
		Expect(
			k8sClient.Create(testCtx, namespace),
		).To(Succeed())

		app1GUID = generateGUID()
		app2GUID = generateGUID()
		process1GUID = generateGUID()
		process2GUID = generateGUID()
	})

	AfterEach(func() {
		Expect(
			k8sClient.Delete(testCtx, namespace),
		).To(Succeed())
	})

	Describe("FetchProcess", func() {
		var (
			namespace1 *corev1.Namespace
			namespace2 *corev1.Namespace
		)

		BeforeEach(func() {
			namespace1 = namespace

			namespace2Name := generateGUID()
			namespace2 = &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace2Name}}
			Expect(k8sClient.Create(context.Background(), namespace2)).To(Succeed())
		})

		AfterEach(func() {
			Expect(k8sClient.Delete(context.Background(), namespace2)).To(Succeed())
		})

		When("on the happy path", func() {
			var (
				cfApp1 *workloadsv1alpha1.CFApp
				cfApp2 *workloadsv1alpha1.CFApp

				cfProcess1 *workloadsv1alpha1.CFProcess
				cfProcess2 *workloadsv1alpha1.CFProcess
			)

			BeforeEach(func() {
				cfApp1 = initializeAppCR("test-app1", app1GUID, namespace1.Name)
				Expect(k8sClient.Create(context.Background(), cfApp1)).To(Succeed())

				cfApp2 = initializeAppCR("test-app2", app2GUID, namespace2.Name)
				Expect(k8sClient.Create(context.Background(), cfApp2)).To(Succeed())

				cfProcess1 = initializeProcessCR(process1GUID, namespace1.Name, app1GUID)
				Expect(k8sClient.Create(context.Background(), cfProcess1)).To(Succeed())

				cfProcess2 = initializeProcessCR(process2GUID, namespace2.Name, app2GUID)
				Expect(k8sClient.Create(context.Background(), cfProcess2)).To(Succeed())
			})

			//AfterEach(func() {
			//	k8sClient.Delete(context.Background(), cfApp1)
			//	k8sClient.Delete(context.Background(), cfApp2)
			//	k8sClient.Delete(context.Background(), cfProcess1)
			//	k8sClient.Delete(context.Background(), cfProcess2)
			//})

			It("returns a Process record for the Process CR we request", func() {
				process, err := processRepo.FetchProcess(testCtx, authInfo, process1GUID)
				Expect(err).NotTo(HaveOccurred())
				By("Returning a record with a matching GUID", func() {
					Expect(process.GUID).To(Equal(process1GUID))
				})
				By("Returning a record with a matching spaceGUID", func() {
					Expect(process.SpaceGUID).To(Equal(namespace1.Name))
				})
				By("Returning a record with a matching appGUID", func() {
					Expect(process.AppGUID).To(Equal(app1GUID))
				})
				By("Returning a record with a matching ProcessType", func() {
					Expect(process.Type).To(Equal(cfProcess1.Spec.ProcessType))
				})
				By("Returning a record with a matching Command", func() {
					Expect(process.Command).To(Equal(cfProcess1.Spec.Command))
				})
				By("Returning a record with a matching Instance Count", func() {
					Expect(process.DesiredInstances).To(Equal(cfProcess1.Spec.DesiredInstances))
				})
				By("Returning a record with a matching MemoryMB", func() {
					Expect(process.MemoryMB).To(Equal(cfProcess1.Spec.MemoryMB))
				})
				By("Returning a record with a matching DiskQuotaMB", func() {
					Expect(process.DiskQuotaMB).To(Equal(cfProcess1.Spec.DiskQuotaMB))
				})
				By("Returning a record with matching Ports", func() {
					Expect(process.Ports).To(Equal(cfProcess1.Spec.Ports))
				})
				By("Returning a record with matching HealthCheck", func() {
					Expect(process.HealthCheck.Type).To(Equal(string(cfProcess1.Spec.HealthCheck.Type)))
					Expect(process.HealthCheck.Data.InvocationTimeoutSeconds).To(Equal(cfProcess1.Spec.HealthCheck.Data.InvocationTimeoutSeconds))
					Expect(process.HealthCheck.Data.TimeoutSeconds).To(Equal(cfProcess1.Spec.HealthCheck.Data.TimeoutSeconds))
					Expect(process.HealthCheck.Data.HTTPEndpoint).To(Equal(cfProcess1.Spec.HealthCheck.Data.HTTPEndpoint))
				})
			})
		})

		When("duplicate Processes exist across namespaces with the same GUIDs", func() {
			var (
				cfApp1 *workloadsv1alpha1.CFApp
				cfApp2 *workloadsv1alpha1.CFApp

				cfProcess1 *workloadsv1alpha1.CFProcess
				cfProcess2 *workloadsv1alpha1.CFProcess
			)

			BeforeEach(func() {
				cfApp1 = initializeAppCR("test-app1", app1GUID, namespace1.Name)
				Expect(k8sClient.Create(context.Background(), cfApp1)).To(Succeed())

				cfApp2 = initializeAppCR("test-app2", app2GUID, namespace2.Name)
				Expect(k8sClient.Create(context.Background(), cfApp2)).To(Succeed())

				cfProcess1 = initializeProcessCR(process1GUID, namespace1.Name, app1GUID)
				Expect(k8sClient.Create(context.Background(), cfProcess1)).To(Succeed())

				cfProcess2 = initializeProcessCR(process1GUID, namespace2.Name, app2GUID)
				Expect(k8sClient.Create(context.Background(), cfProcess2)).To(Succeed())
			})

			//AfterEach(func() {
			//	k8sClient.Delete(context.Background(), cfApp1)
			//	k8sClient.Delete(context.Background(), cfApp2)
			//	k8sClient.Delete(context.Background(), cfProcess1)
			//	k8sClient.Delete(context.Background(), cfProcess2)
			//})

			It("returns an error", func() {
				_, err := processRepo.FetchProcess(testCtx, authInfo, process1GUID)
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError("duplicate processes exist"))
			})
		})

		When("no Processes exist", func() {
			It("returns an error", func() {
				_, err := processRepo.FetchProcess(testCtx, authInfo, "i don't exist")
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(NotFoundError{ResourceType: "Process"}))
			})
		})
	})

	Describe("FetchProcessesList", func() {
		var (
			cfApp1 *workloadsv1alpha1.CFApp
			cfApp2 *workloadsv1alpha1.CFApp

			cfProcess1 *workloadsv1alpha1.CFProcess
			cfProcess2 *workloadsv1alpha1.CFProcess
		)

		BeforeEach(func() {
			cfApp1 = initializeAppCR("test-app1", app1GUID, namespace.Name)
			Expect(k8sClient.Create(testCtx, cfApp1)).To(Succeed())

			cfApp2 = initializeAppCR("test-app2", app2GUID, namespace.Name)
			Expect(k8sClient.Create(testCtx, cfApp2)).To(Succeed())

			cfProcess1 = initializeProcessCR(process1GUID, namespace.Name, app1GUID)
			Expect(k8sClient.Create(testCtx, cfProcess1)).To(Succeed())

			cfProcess2 = initializeProcessCR(process2GUID, namespace.Name, app1GUID)
			Expect(k8sClient.Create(testCtx, cfProcess2)).To(Succeed())
		})

		When("on the happy path", func() {
			When("spaceGUID is not empty", func() {
				It("returns Process records for the AppGUID we request", func() {
					processes, err := processRepo.FetchProcessList(testCtx, authInfo, FetchProcessListMessage{AppGUID: []string{app1GUID}, SpaceGUID: namespace.Name})
					Expect(err).NotTo(HaveOccurred())
					Expect(len(processes)).To(Equal(2))
					By("returning a process record for each process of the app", func() {
						for _, processRecord := range processes {
							recordMatchesOneProcess := processRecord.GUID == process1GUID || processRecord.GUID == process2GUID
							Expect(recordMatchesOneProcess).To(BeTrue(), "ProcessRecord GUID did not match one of the expected processes")
						}
					})
				})
			})

			When("spaceGUID is empty", func() {
				It("returns Process records for the AppGUID we request", func() {
					processes, err := processRepo.FetchProcessList(testCtx, authInfo, FetchProcessListMessage{AppGUID: []string{app1GUID}})
					Expect(err).NotTo(HaveOccurred())
					Expect(len(processes)).To(Equal(2))
					By("returning a process record for each process of the app", func() {
						for _, processRecord := range processes {
							recordMatchesOneProcess := processRecord.GUID == process1GUID || processRecord.GUID == process2GUID
							Expect(recordMatchesOneProcess).To(BeTrue(), "ProcessRecord GUID did not match one of the expected processes")
						}
					})
				})
			})
		})

		When("no Processes exist for an app", func() {
			It("returns an empty list", func() {
				processes, err := processRepo.FetchProcessList(testCtx, authInfo, FetchProcessListMessage{AppGUID: []string{app2GUID}, SpaceGUID: namespace.Name})
				Expect(err).ToNot(HaveOccurred())
				Expect(processes).To(BeEmpty())
				Expect(processes).ToNot(BeNil())
			})
		})

		When("the app does not exist", func() {
			It("returns an empty list", func() {
				processes, err := processRepo.FetchProcessList(testCtx, authInfo, FetchProcessListMessage{AppGUID: []string{"I dont exist"}, SpaceGUID: namespace.Name})
				Expect(err).ToNot(HaveOccurred())
				Expect(processes).To(BeEmpty())
				Expect(processes).ToNot(BeNil())
			})
		})
	})

	Describe("ScaleProcess", func() {
		var (
			cfApp               *workloadsv1alpha1.CFApp
			cfProcess           *workloadsv1alpha1.CFProcess
			scaleProcessMessage *ProcessScaleMessage
		)

		BeforeEach(func() {
			cfApp = initializeAppCR("test-app1", app1GUID, namespace.Name)
			Expect(k8sClient.Create(context.Background(), cfApp)).To(Succeed())

			cfProcess = initializeProcessCR(process1GUID, namespace.Name, app1GUID)
			Expect(k8sClient.Create(context.Background(), cfProcess)).To(Succeed())

			scaleProcessMessage = &ProcessScaleMessage{
				GUID:               process1GUID,
				SpaceGUID:          namespace.Name,
				ProcessScaleValues: ProcessScaleValues{},
			}
		})

		When("on the happy path", func() {
			var (
				instanceScale int
				diskScaleMB   int64
				memoryScaleMB int64
			)

			BeforeEach(func() {
				instanceScale = 7
				diskScaleMB = 80
				memoryScaleMB = 900
			})

			DescribeTable("calling ScaleProcess with a set of scale values returns an updated CFProcess record",
				func(instances *int, diskMB, memoryMB *int64) {
					scaleProcessMessage.ProcessScaleValues = ProcessScaleValues{
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

			It("eventually updates the scale of the CFProcess CR", func() {
				scaleProcessMessage.ProcessScaleValues = ProcessScaleValues{
					Instances: &instanceScale,
					MemoryMB:  &memoryScaleMB,
					DiskMB:    &diskScaleMB,
				}
				_, err := processRepo.ScaleProcess(testCtx, authInfo, *scaleProcessMessage)
				Expect(err).ToNot(HaveOccurred())
				var updatedCFProcess workloadsv1alpha1.CFProcess

				Eventually(func() int {
					lookupKey := types.NamespacedName{Name: process1GUID, Namespace: namespace.Name}
					err := k8sClient.Get(context.Background(), lookupKey, &updatedCFProcess)
					if err != nil {
						return 0
					}
					return updatedCFProcess.Spec.DesiredInstances
				}, timeCheckThreshold*time.Second).Should(Equal(instanceScale), "instance scale was not updated")

				By("updating the CFProcess CR instance scale appropriately", func() {
					Expect(updatedCFProcess.Spec.DesiredInstances).To(Equal(instanceScale))
				})

				By("updating the CFProcess CR disk scale appropriately", func() {
					Expect(updatedCFProcess.Spec.DiskQuotaMB).To(Equal(diskScaleMB))
				})

				By("updating the CFProcess CR memory scale appropriately", func() {
					Expect(updatedCFProcess.Spec.MemoryMB).To(Equal(memoryScaleMB))
				})
			})
		})

		When("the process does not exist", func() {
			It("returns an error", func() {
				scaleProcessMessage.GUID = "i-dont-exist"
				_, err := processRepo.ScaleProcess(testCtx, authInfo, *scaleProcessMessage)
				Expect(err).To(HaveOccurred())
			})
		})
	})

	Describe("CreateProcess", func() {
		It("creates a CFProcess resource", func() {
			Expect(
				processRepo.CreateProcess(testCtx, authInfo, ProcessCreateMessage{
					AppGUID:     app1GUID,
					SpaceGUID:   namespace.Name,
					Type:        "web",
					Command:     "start-web",
					DiskQuotaMB: 123,
					Healthcheck: HealthCheck{
						Type: "http",
						Data: HealthCheckData{
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
			Eventually(func() []workloadsv1alpha1.CFProcess {
				Expect(
					k8sClient.List(testCtx, &list, client.InNamespace(namespace.Name)),
				).To(Succeed())
				return list.Items
			}).Should(HaveLen(1))

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

	Describe("FetchProcessByAppTypeAndSpace", func() {
		const (
			processType = "thingy"
		)

		When("there is a matching process", func() {
			BeforeEach(func() {
				cfProcess := &workloadsv1alpha1.CFProcess{
					ObjectMeta: metav1.ObjectMeta{
						Name:      process1GUID,
						Namespace: namespace.Name,
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
				processRecord, err := processRepo.FetchProcessByAppTypeAndSpace(testCtx, authInfo, app1GUID, processType, namespace.Name)
				Expect(err).NotTo(HaveOccurred())
				Expect(processRecord).To(MatchAllFields(Fields{
					"GUID":             Equal(process1GUID),
					"SpaceGUID":        Equal(namespace.Name),
					"AppGUID":          Equal(app1GUID),
					"Type":             Equal(processType),
					"Command":          Equal("the-command"),
					"DesiredInstances": Equal(1),
					"MemoryMB":         BeEquivalentTo(2),
					"DiskQuotaMB":      BeEquivalentTo(3),
					"Ports":            Equal([]int32{8080}),
					"HealthCheck": Equal(HealthCheck{
						Type: "http",
						Data: HealthCheckData{
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
				_, err := processRepo.FetchProcessByAppTypeAndSpace(testCtx, authInfo, app1GUID, processType, namespace.Name)
				Expect(err).To(MatchError(NotFoundError{ResourceType: "Process"}))
			})
		})
	})

	Describe("PatchProcess", func() {
		When("the app already has a process with the given type", func() {
			var (
				cfProcess *workloadsv1alpha1.CFProcess
				message   ProcessPatchMessage
			)

			BeforeEach(func() {
				cfProcess = &workloadsv1alpha1.CFProcess{
					ObjectMeta: metav1.ObjectMeta{
						Name:      process1GUID,
						Namespace: namespace.Name,
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
					message = ProcessPatchMessage{
						ProcessGUID:                         process1GUID,
						SpaceGUID:                           namespace.Name,
						Command:                             stringPointer("start-web"),
						HealthcheckType:                     stringPointer("http"),
						HealthCheckHTTPEndpoint:             stringPointer("/healthz"),
						HealthCheckInvocationTimeoutSeconds: int64Pointer(20),
						HealthCheckTimeoutSeconds:           int64Pointer(10),
						DesiredInstances:                    intPointer(42),
						MemoryMB:                            int64Pointer(456),
						DiskQuotaMB:                         int64Pointer(123),
					}
				})

				It("updates all fields on the existing CFProcess resource", func() {
					Expect(
						processRepo.PatchProcess(testCtx, authInfo, message),
					).To(Succeed())

					var process workloadsv1alpha1.CFProcess
					Eventually(func() workloadsv1alpha1.CFProcessSpec {
						Expect(
							k8sClient.Get(testCtx, types.NamespacedName{Name: process1GUID, Namespace: namespace.Name}, &process),
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
					message = ProcessPatchMessage{
						ProcessGUID:               process1GUID,
						SpaceGUID:                 namespace.Name,
						Command:                   stringPointer("new-command"),
						HealthCheckTimeoutSeconds: int64Pointer(42),
						DesiredInstances:          intPointer(5),
						MemoryMB:                  int64Pointer(123),
					}
				})

				It("patches only the provided fields on the Process", func() {
					Expect(
						processRepo.PatchProcess(testCtx, authInfo, message),
					).To(Succeed())

					var process workloadsv1alpha1.CFProcess
					Eventually(func() workloadsv1alpha1.CFProcessSpec {
						Expect(
							k8sClient.Get(testCtx, types.NamespacedName{Name: process1GUID, Namespace: namespace.Name}, &process),
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
