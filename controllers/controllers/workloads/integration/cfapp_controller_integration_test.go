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
	When("a new CFApp resource is created", func() {
		const (
			cfAppGUID = "test-app-guid"
			namespace = "default"
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
					Namespace: namespace,
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

			cfAppLookupKey := types.NamespacedName{Name: cfAppGUID, Namespace: namespace}
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
	When("a CFApp resource exists", func() {
		var (
			namespaceGUID string
			cfAppGUID     string
			cfBuildGUID   string
			cfPackageGUID string
			newNamespace  *corev1.Namespace
			cfApp         *workloadsv1alpha1.CFApp
			cfPackage     *workloadsv1alpha1.CFPackage
			cfBuild       *workloadsv1alpha1.CFBuild
		)

		BeforeEach(func() {
			namespaceGUID = GenerateGUID()
			cfAppGUID = GenerateGUID()
			cfPackageGUID = GenerateGUID()
			cfBuildGUID = GenerateGUID()

			beforeCtx := context.Background()

			newNamespace = BuildNamespaceObject(namespaceGUID)
			Expect(k8sClient.Create(beforeCtx, newNamespace)).To(Succeed())

			cfApp = BuildCFAppCRObject(cfAppGUID, namespaceGUID)
			Expect(k8sClient.Create(beforeCtx, cfApp)).To(Succeed())

			cfPackage = BuildCFPackageCRObject(cfPackageGUID, namespaceGUID, cfAppGUID)
			Expect(k8sClient.Create(beforeCtx, cfPackage)).To(Succeed())

			cfBuild = BuildCFBuildObject(cfBuildGUID, namespaceGUID, cfPackageGUID, cfAppGUID)
			Expect(k8sClient.Create(beforeCtx, cfBuild)).To(Succeed())
		})

		When("currentDropletRef is set", func() {

			When("referenced build/droplet exist", func() {
				const (
					processTypeWeb           = "web"
					processTypeWebCommand    = "bundle exec rackup config.ru -p $PORT -o 0.0.0.0"
					processTypeWorker        = "worker"
					processTypeWorkerCommand = "bundle exec rackup config.ru"
					port8080                 = 8080
					port9000                 = 9000
				)
				When("CFProcesses do not exist for the app", func() {

					var createdCFBuild *workloadsv1alpha1.CFBuild

					BeforeEach(func() {
						beforeCtx := context.Background()
						cfBuildLookupKey := types.NamespacedName{Name: cfBuildGUID, Namespace: namespaceGUID}
						createdCFBuild = new(workloadsv1alpha1.CFBuild)
						Eventually(func() []metav1.Condition {
							err := k8sClient.Get(beforeCtx, cfBuildLookupKey, createdCFBuild)
							if err != nil {
								return nil
							}
							return createdCFBuild.Status.Conditions
						}, 10*time.Second, 250*time.Millisecond).ShouldNot(BeEmpty(), "could not retrieve the cfbuild")

						createdCFBuild.Status.BuildDropletStatus = &workloadsv1alpha1.BuildDropletStatus{
							Registry: workloadsv1alpha1.Registry{
								Image:            "image/registry/url",
								ImagePullSecrets: nil,
							},
							Stack: "cflinuxfs3",
							ProcessTypes: []workloadsv1alpha1.ProcessType{
								{
									Type:    processTypeWeb,
									Command: processTypeWebCommand,
								},
								{
									Type:    processTypeWorker,
									Command: processTypeWorkerCommand,
								},
							},
							Ports: []int32{port8080, port9000},
						}
						Expect(k8sClient.Status().Update(beforeCtx, createdCFBuild)).To(Succeed())

						baseCFApp := &workloadsv1alpha1.CFApp{
							ObjectMeta: metav1.ObjectMeta{
								Name:      cfAppGUID,
								Namespace: namespaceGUID,
							},
						}
						patchedCFApp := baseCFApp.DeepCopy()
						patchedCFApp.Spec.CurrentDropletRef = corev1.LocalObjectReference{Name: cfBuildGUID}
						Expect(k8sClient.Patch(beforeCtx, patchedCFApp, client.MergeFrom(baseCFApp))).To(Succeed())
					})

					It("should eventually create CFProcess for each process listed on the droplet", func() {
						testCtx := context.Background()
						droplet := createdCFBuild.Status.BuildDropletStatus
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

				When("CFProcess exists for the app", func() {

					var (
						cfProcessForTypeWebGUID string
						cfProcessForTypeWeb     *workloadsv1alpha1.CFProcess
						createdCFBuild          *workloadsv1alpha1.CFBuild
					)

					BeforeEach(func() {
						beforeCtx := context.Background()
						cfProcessForTypeWebGUID = GenerateGUID()
						cfProcessForTypeWeb = BuildCFProcessCRObject(cfProcessForTypeWebGUID, namespaceGUID, cfAppGUID, processTypeWeb, processTypeWebCommand)
						Expect(k8sClient.Create(beforeCtx, cfProcessForTypeWeb)).To(Succeed())

						cfBuildLookupKey := types.NamespacedName{Name: cfBuildGUID, Namespace: namespaceGUID}
						createdCFBuild = new(workloadsv1alpha1.CFBuild)
						Eventually(func() []metav1.Condition {
							err := k8sClient.Get(beforeCtx, cfBuildLookupKey, createdCFBuild)
							if err != nil {
								return nil
							}
							return createdCFBuild.Status.Conditions
						}, 10*time.Second, 250*time.Millisecond).ShouldNot(BeEmpty(), "could not retrieve the cfbuild")

						createdCFBuild.Status.BuildDropletStatus = &workloadsv1alpha1.BuildDropletStatus{
							Registry: workloadsv1alpha1.Registry{
								Image:            "image/registry/url",
								ImagePullSecrets: nil,
							},
							Stack: "cflinuxfs3",
							ProcessTypes: []workloadsv1alpha1.ProcessType{
								{
									Type:    processTypeWeb,
									Command: processTypeWebCommand,
								},
								{
									Type:    processTypeWorker,
									Command: processTypeWorkerCommand,
								},
							},
							Ports: []int32{port8080, port9000},
						}
						Expect(k8sClient.Status().Update(beforeCtx, createdCFBuild)).To(Succeed())

						baseCFApp := &workloadsv1alpha1.CFApp{
							ObjectMeta: metav1.ObjectMeta{
								Name:      cfAppGUID,
								Namespace: namespaceGUID,
							},
						}
						patchedCFApp := baseCFApp.DeepCopy()
						patchedCFApp.Spec.CurrentDropletRef = corev1.LocalObjectReference{Name: cfBuildGUID}
						Expect(k8sClient.Patch(beforeCtx, patchedCFApp, client.MergeFrom(baseCFApp))).To(Succeed())
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
		})

	})
})
