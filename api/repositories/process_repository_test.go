package repositories_test

import (
	"context"
	"time"

	. "code.cloudfoundry.org/cf-k8s-api/repositories"
	workloadsv1alpha1 "code.cloudfoundry.org/cf-k8s-controllers/apis/workloads/v1alpha1"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("ProcessRepository", func() {
	var (
		testCtx     context.Context
		processRepo *ProcessRepository
		client      client.Client
	)

	BeforeEach(func() {
		testCtx = context.Background()

		processRepo = new(ProcessRepository)
		var err error
		client, err = BuildCRClient(k8sConfig)
		Expect(err).ToNot(HaveOccurred())
	})

	Describe("FetchProcess", func() {
		var (
			namespace1 *corev1.Namespace
			namespace2 *corev1.Namespace
		)

		BeforeEach(func() {
			beforeCtx := context.Background()
			namespace1Name := generateGUID()
			namespace1 = &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace1Name}}
			Expect(k8sClient.Create(beforeCtx, namespace1)).To(Succeed())

			namespace2Name := generateGUID()
			namespace2 = &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace2Name}}
			Expect(k8sClient.Create(beforeCtx, namespace2)).To(Succeed())
		})

		AfterEach(func() {
			afterCtx := context.Background()
			k8sClient.Delete(afterCtx, namespace1)
			k8sClient.Delete(afterCtx, namespace2)
		})

		When("on the happy path", func() {
			var (
				app1GUID string
				app2GUID string
				cfApp1   *workloadsv1alpha1.CFApp
				cfApp2   *workloadsv1alpha1.CFApp

				process1GUID string
				process2GUID string
				cfProcess1   *workloadsv1alpha1.CFProcess
				cfProcess2   *workloadsv1alpha1.CFProcess
			)

			BeforeEach(func() {
				app1GUID = generateGUID()
				app2GUID = generateGUID()
				cfApp1 = initializeAppCR("test-app1", app1GUID, namespace1.Name)
				Expect(k8sClient.Create(context.Background(), cfApp1)).To(Succeed())

				cfApp2 = initializeAppCR("test-app2", app2GUID, namespace2.Name)
				Expect(k8sClient.Create(context.Background(), cfApp2)).To(Succeed())

				process1GUID = generateGUID()
				cfProcess1 = initializeProcessCR(process1GUID, namespace1.Name, app1GUID)
				Expect(k8sClient.Create(context.Background(), cfProcess1)).To(Succeed())

				process2GUID = generateGUID()
				cfProcess2 = initializeProcessCR(process2GUID, namespace2.Name, app2GUID)
				Expect(k8sClient.Create(context.Background(), cfProcess2)).To(Succeed())

			})

			AfterEach(func() {
				k8sClient.Delete(context.Background(), cfApp1)
				k8sClient.Delete(context.Background(), cfApp2)
				k8sClient.Delete(context.Background(), cfProcess1)
				k8sClient.Delete(context.Background(), cfProcess2)
			})

			It("returns a Process record for the Process CR we request", func() {
				process, err := processRepo.FetchProcess(testCtx, client, process1GUID)
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
					Expect(process.Instances).To(Equal(cfProcess1.Spec.DesiredInstances))
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
				app1GUID string
				app2GUID string
				cfApp1   *workloadsv1alpha1.CFApp
				cfApp2   *workloadsv1alpha1.CFApp

				processGUID string
				cfProcess1  *workloadsv1alpha1.CFProcess
				cfProcess2  *workloadsv1alpha1.CFProcess
			)

			BeforeEach(func() {
				app1GUID = generateGUID()
				app2GUID = generateGUID()
				cfApp1 = initializeAppCR("test-app1", app1GUID, namespace1.Name)
				Expect(k8sClient.Create(context.Background(), cfApp1)).To(Succeed())

				cfApp2 = initializeAppCR("test-app2", app2GUID, namespace2.Name)
				Expect(k8sClient.Create(context.Background(), cfApp2)).To(Succeed())

				processGUID = generateGUID()
				cfProcess1 = initializeProcessCR(processGUID, namespace1.Name, app1GUID)
				Expect(k8sClient.Create(context.Background(), cfProcess1)).To(Succeed())

				cfProcess2 = initializeProcessCR(processGUID, namespace2.Name, app2GUID)
				Expect(k8sClient.Create(context.Background(), cfProcess2)).To(Succeed())

			})

			AfterEach(func() {
				k8sClient.Delete(context.Background(), cfApp1)
				k8sClient.Delete(context.Background(), cfApp2)
				k8sClient.Delete(context.Background(), cfProcess1)
				k8sClient.Delete(context.Background(), cfProcess2)
			})

			It("returns an error", func() {
				_, err := processRepo.FetchProcess(testCtx, client, processGUID)
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError("duplicate processes exist"))
			})
		})

		When("no Processes exist", func() {
			It("returns an error", func() {
				_, err := processRepo.FetchProcess(testCtx, client, "i don't exist")
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(NotFoundError{ResourceType: "Process"}))
			})
		})
	})

	Describe("FetchProcessesForApp", func() {
		var (
			namespaceGUID string
			namespace     *corev1.Namespace

			app1GUID string
			app2GUID string
			cfApp1   *workloadsv1alpha1.CFApp
			cfApp2   *workloadsv1alpha1.CFApp

			process1GUID string
			process2GUID string
			cfProcess1   *workloadsv1alpha1.CFProcess
			cfProcess2   *workloadsv1alpha1.CFProcess
		)

		BeforeEach(func() {
			beforeCtx := context.Background()
			namespaceGUID = generateGUID()
			namespace = &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespaceGUID}}
			Expect(k8sClient.Create(beforeCtx, namespace)).To(Succeed())

			app1GUID = generateGUID()
			app2GUID = generateGUID()
			cfApp1 = initializeAppCR("test-app1", app1GUID, namespaceGUID)
			Expect(k8sClient.Create(beforeCtx, cfApp1)).To(Succeed())

			cfApp2 = initializeAppCR("test-app2", app2GUID, namespaceGUID)
			Expect(k8sClient.Create(beforeCtx, cfApp2)).To(Succeed())

			process1GUID = generateGUID()
			cfProcess1 = initializeProcessCR(process1GUID, namespaceGUID, app1GUID)
			Expect(k8sClient.Create(beforeCtx, cfProcess1)).To(Succeed())

			process2GUID = generateGUID()
			cfProcess2 = initializeProcessCR(process2GUID, namespaceGUID, app1GUID)
			Expect(k8sClient.Create(beforeCtx, cfProcess2)).To(Succeed())
		})

		AfterEach(func() {
			afterCtx := context.Background()
			k8sClient.Delete(afterCtx, cfApp1)
			k8sClient.Delete(afterCtx, cfApp2)
			k8sClient.Delete(afterCtx, cfProcess1)
			k8sClient.Delete(afterCtx, cfProcess2)
			k8sClient.Delete(afterCtx, namespace)
		})

		When("on the happy path", func() {

			It("returns Process records for the AppGUID we request", func() {
				processes, err := processRepo.FetchProcessesForApp(testCtx, client, app1GUID, namespaceGUID)
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

		When("no Processes exist for an app", func() {
			It("returns an empty list", func() {
				processes, err := processRepo.FetchProcessesForApp(testCtx, client, app2GUID, namespaceGUID)
				Expect(err).ToNot(HaveOccurred())
				Expect(processes).To(BeEmpty())
				Expect(processes).ToNot(BeNil())
			})
		})

		When("the app does not exist", func() {
			It("returns an empty list", func() {
				processes, err := processRepo.FetchProcessesForApp(testCtx, client, "I don't exist", namespaceGUID)
				Expect(err).ToNot(HaveOccurred())
				Expect(processes).To(BeEmpty())
				Expect(processes).ToNot(BeNil())
			})
		})
	})

	Describe("ScaleProcess", func() {
		var (
			namespace *corev1.Namespace

			appGUID string
			cfApp   *workloadsv1alpha1.CFApp

			processGUID string
			cfProcess   *workloadsv1alpha1.CFProcess

			scaleProcessMessage *ScaleProcessMessage
		)

		BeforeEach(func() {
			beforeCtx := context.Background()
			namespaceName := generateGUID()
			namespace = &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespaceName}}
			Expect(k8sClient.Create(beforeCtx, namespace)).To(Succeed())

			appGUID = generateGUID()
			cfApp = initializeAppCR("test-app1", appGUID, namespace.Name)
			Expect(k8sClient.Create(context.Background(), cfApp)).To(Succeed())

			processGUID = generateGUID()
			cfProcess = initializeProcessCR(processGUID, namespace.Name, appGUID)
			Expect(k8sClient.Create(context.Background(), cfProcess)).To(Succeed())

			scaleProcessMessage = &ScaleProcessMessage{
				GUID:                processGUID,
				SpaceGUID:           namespace.Name,
				ProcessScaleMessage: ProcessScaleMessage{},
			}
		})

		AfterEach(func() {
			afterCtx := context.Background()
			k8sClient.Delete(afterCtx, namespace)
			k8sClient.Delete(afterCtx, cfApp)
			k8sClient.Delete(afterCtx, cfProcess)
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
					scaleProcessMessage.ProcessScaleMessage = ProcessScaleMessage{
						Instances: instances,
						DiskMB:    diskMB,
						MemoryMB:  memoryMB,
					}
					scaleProcessRecord, scaleProcessErr := processRepo.ScaleProcess(context.Background(), client, *scaleProcessMessage)
					Expect(scaleProcessErr).ToNot(HaveOccurred())
					if instances != nil {
						Expect(scaleProcessRecord.Instances).To(Equal(*instances))
					} else {
						Expect(scaleProcessRecord.Instances).To(Equal(cfProcess.Spec.DesiredInstances))
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
				scaleProcessMessage.ProcessScaleMessage = ProcessScaleMessage{
					Instances: &instanceScale,
					MemoryMB:  &memoryScaleMB,
					DiskMB:    &diskScaleMB,
				}
				_, err := processRepo.ScaleProcess(testCtx, client, *scaleProcessMessage)
				Expect(err).ToNot(HaveOccurred())
				var updatedCFProcess workloadsv1alpha1.CFProcess

				Eventually(func() int {
					lookupKey := types.NamespacedName{Name: processGUID, Namespace: namespace.Name}
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
				_, err := processRepo.ScaleProcess(testCtx, client, *scaleProcessMessage)
				Expect(err).To(HaveOccurred())
			})
		})
	})
})
