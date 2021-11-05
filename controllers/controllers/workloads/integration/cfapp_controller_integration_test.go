package integration_test

import (
	"context"
	"time"

	workloadsv1alpha1 "code.cloudfoundry.org/cf-k8s-controllers/controllers/apis/workloads/v1alpha1"
	. "code.cloudfoundry.org/cf-k8s-controllers/controllers/controllers/workloads/testutils"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("CFAppReconciler", func() {
	var (
		namespaceGUID string
		ns            *corev1.Namespace
	)

	BeforeEach(func() {
		namespaceGUID = GenerateGUID()
		ns = createNamespace(context.Background(), k8sClient, namespaceGUID)
	})

	AfterEach(func() {
		k8sClient.Delete(context.Background(), ns)
	})

	When("a new CFApp resource is created", func() {
		const (
			cfAppGUID = "test-app-guid"
		)

		It("sets its status.conditions", func() {
			ctx := context.Background()
			cfApp := &workloadsv1alpha1.CFApp{
				TypeMeta: metav1.TypeMeta{
					Kind:       "CFApp",
					APIVersion: workloadsv1alpha1.GroupVersion.Identifier(),
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      cfAppGUID,
					Namespace: namespaceGUID,
				},
				Spec: workloadsv1alpha1.CFAppSpec{
					Name:         "test-app",
					DesiredState: "STOPPED",
					Lifecycle: workloadsv1alpha1.Lifecycle{
						Type: "buildpack",
					},
				},
			}
			Expect(k8sClient.Create(ctx, cfApp)).To(Succeed())

			cfAppLookupKey := types.NamespacedName{Name: cfAppGUID, Namespace: namespaceGUID}
			createdCFApp := new(workloadsv1alpha1.CFApp)

			Eventually(func() []metav1.Condition {
				err := k8sClient.Get(ctx, cfAppLookupKey, createdCFApp)
				if err != nil {
					return nil
				}
				return createdCFApp.Status.Conditions
			}, 10*time.Second, 250*time.Millisecond).ShouldNot(BeEmpty())

			runningConditionFalse := meta.IsStatusConditionFalse(createdCFApp.Status.Conditions, "Running")
			Expect(runningConditionFalse).To(BeTrue())

			restartingConditionFalse := meta.IsStatusConditionFalse(createdCFApp.Status.Conditions, "Restarting")
			Expect(restartingConditionFalse).To(BeTrue())
		})
	})
	When("a CFApp resource exists and", func() {
		const (
			processTypeWeb           = "web"
			processTypeWebCommand    = "bundle exec rackup config.ru -p $PORT -o 0.0.0.0"
			processTypeWorker        = "worker"
			processTypeWorkerCommand = "bundle exec rackup config.ru"
			port8080                 = 8080
			port9000                 = 9000
		)

		var (
			cfAppGUID     string
			cfBuildGUID   string
			cfPackageGUID string
			cfApp         *workloadsv1alpha1.CFApp
			cfPackage     *workloadsv1alpha1.CFPackage
			cfBuild       *workloadsv1alpha1.CFBuild
		)

		BeforeEach(func() {
			cfAppGUID = GenerateGUID()
			cfPackageGUID = GenerateGUID()
			cfBuildGUID = GenerateGUID()

			beforeCtx := context.Background()

			cfApp = BuildCFAppCRObject(cfAppGUID, namespaceGUID)
			Expect(k8sClient.Create(beforeCtx, cfApp)).To(Succeed())

			cfPackage = BuildCFPackageCRObject(cfPackageGUID, namespaceGUID, cfAppGUID)
			Expect(k8sClient.Create(beforeCtx, cfPackage)).To(Succeed())

			cfBuild = BuildCFBuildObject(cfBuildGUID, namespaceGUID, cfPackageGUID, cfAppGUID)
			dropletProcessTypeMap := map[string]string{
				processTypeWeb:    processTypeWebCommand,
				processTypeWorker: processTypeWorkerCommand,
			}
			dropletPorts := []int32{port8080, port9000}
			buildDropletStatus := BuildCFBuildDropletStatusObject(dropletProcessTypeMap, dropletPorts)
			cfBuild = createBuildWithDroplet(beforeCtx, k8sClient, cfBuild, buildDropletStatus)
		})

		When("currentDropletRef is set and", func() {

			When("the referenced build/droplet exists", func() {
				When("CFProcesses do not exist for the app", func() {
					BeforeEach(func() {
						patchAppWithDroplet(context.Background(), k8sClient, cfAppGUID, namespaceGUID, cfBuildGUID)
					})

					It("should eventually create CFProcess for each process listed on the droplet", func() {
						testCtx := context.Background()
						droplet := cfBuild.Status.BuildDropletStatus
						processTypes := droplet.ProcessTypes
						for _, process := range processTypes {
							cfProcessList := workloadsv1alpha1.CFProcessList{}
							labelSelectorMap := labels.Set{
								CFAppLabelKey:         cfAppGUID,
								CFProcessTypeLabelKey: process.Type,
							}
							selector, selectorValidationErr := labelSelectorMap.AsValidatedSelector()
							Expect(selectorValidationErr).To(BeNil())
							Eventually(func() []workloadsv1alpha1.CFProcess {
								_ = k8sClient.List(testCtx, &cfProcessList, &client.ListOptions{LabelSelector: selector, Namespace: cfApp.Namespace})
								return cfProcessList.Items
							}, 10*time.Second, 250*time.Millisecond).Should(HaveLen(1), "expected CFProcess to eventually be created")
							createdCFProcess := cfProcessList.Items[0]
							Expect(createdCFProcess.Spec.Command).To(Equal(process.Command), "cfprocess command does not match with droplet command")
							Expect(createdCFProcess.Spec.AppRef.Name).To(Equal(cfAppGUID), "cfprocess app ref does not match app-guid")
							Expect(createdCFProcess.Spec.Ports).To(Equal(droplet.Ports), "cfprocess ports does not match ports on droplet")
						}
					})
				})

				When("CFProcesses exist for the app", func() {

					var (
						cfProcessForTypeWebGUID string
						cfProcessForTypeWeb     *workloadsv1alpha1.CFProcess
					)

					BeforeEach(func() {
						beforeCtx := context.Background()
						cfProcessForTypeWebGUID = GenerateGUID()
						cfProcessForTypeWeb = BuildCFProcessCRObject(cfProcessForTypeWebGUID, namespaceGUID, cfAppGUID, processTypeWeb, processTypeWebCommand)
						Expect(k8sClient.Create(beforeCtx, cfProcessForTypeWeb)).To(Succeed())

						patchAppWithDroplet(beforeCtx, k8sClient, cfAppGUID, namespaceGUID, cfBuildGUID)
					})

					It("should eventually create CFProcess for only the missing processTypes", func() {
						testCtx := context.Background()

						//Checking for worker type first ensures that we wait long enough for processes to be created.
						cfProcessList := workloadsv1alpha1.CFProcessList{}
						labelSelectorMap := labels.Set{
							CFAppLabelKey:         cfAppGUID,
							CFProcessTypeLabelKey: processTypeWorker,
						}
						selector, selectorValidationErr := labelSelectorMap.AsValidatedSelector()
						Expect(selectorValidationErr).To(BeNil())
						Eventually(func() []workloadsv1alpha1.CFProcess {
							_ = k8sClient.List(testCtx, &cfProcessList, &client.ListOptions{LabelSelector: selector, Namespace: cfApp.Namespace})
							return cfProcessList.Items
						}, 10*time.Second, 250*time.Millisecond).Should(HaveLen(1), "Count of CFProcess is not equal to 1")

						cfProcessList = workloadsv1alpha1.CFProcessList{}
						labelSelectorMap = labels.Set{
							CFAppLabelKey:         cfAppGUID,
							CFProcessTypeLabelKey: processTypeWorker,
						}
						selector, selectorValidationErr = labelSelectorMap.AsValidatedSelector()
						Expect(selectorValidationErr).To(BeNil())
						Expect(k8sClient.List(testCtx, &cfProcessList, &client.ListOptions{LabelSelector: selector, Namespace: cfApp.Namespace})).To(Succeed())
						Expect(cfProcessList.Items).Should(HaveLen(1), "Count of CFProcess is not equal to 1")
					})
				})

			})

			When("the droplet has no ports set", func() {
				var (
					otherBuildGUID string
					otherCFBuild   *workloadsv1alpha1.CFBuild
				)

				BeforeEach(func() {
					beforeCtx := context.Background()

					otherBuildGUID = GenerateGUID()
					otherCFBuild = BuildCFBuildObject(otherBuildGUID, namespaceGUID, cfPackageGUID, cfAppGUID)
					dropletProcessTypeMap := map[string]string{
						processTypeWeb: processTypeWebCommand,
					}
					dropletPorts := []int32{}
					buildDropletStatus := BuildCFBuildDropletStatusObject(dropletProcessTypeMap, dropletPorts)
					otherCFBuild = createBuildWithDroplet(beforeCtx, k8sClient, otherCFBuild, buildDropletStatus)

					patchAppWithDroplet(beforeCtx, k8sClient, cfAppGUID, namespaceGUID, cfBuildGUID)
				})

				It("should eventually create CFProcess with empty ports and a healthCheck type of \"process\"", func() {
					testCtx := context.Background()
					droplet := cfBuild.Status.BuildDropletStatus
					processTypes := droplet.ProcessTypes
					for _, process := range processTypes {
						cfProcessList := workloadsv1alpha1.CFProcessList{}
						labelSelectorMap := labels.Set{
							CFAppLabelKey:         cfAppGUID,
							CFProcessTypeLabelKey: process.Type,
						}
						selector, selectorValidationErr := labelSelectorMap.AsValidatedSelector()
						Expect(selectorValidationErr).To(BeNil())
						Eventually(func() []workloadsv1alpha1.CFProcess {
							_ = k8sClient.List(testCtx, &cfProcessList, &client.ListOptions{LabelSelector: selector, Namespace: cfApp.Namespace})
							return cfProcessList.Items
						}, 10*time.Second, 250*time.Millisecond).Should(HaveLen(1), "expected CFProcess to eventually be created")
						createdCFProcess := cfProcessList.Items[0]
						Expect(createdCFProcess.Spec.Ports).To(Equal(droplet.Ports), "cfprocess ports does not match ports on droplet")
						Expect(string(createdCFProcess.Spec.HealthCheck.Type)).To(Equal("process"))
					}
				})
			})
		})

	})
})
