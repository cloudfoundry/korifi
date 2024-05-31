package build_test

import (
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/tools/k8s"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("CFBuildReconciler", func() {
	var (
		cfApp     *korifiv1alpha1.CFApp
		cfPackage *korifiv1alpha1.CFPackage
		cfBuild   *korifiv1alpha1.CFBuild
	)

	reconciledBuilds := func() map[string]int {
		result := map[string]int{}
		reconciledBuildsSync.Range(func(k, v any) bool {
			result[k.(string)] = v.(int)
			return true
		})
		return result
	}

	buildCleanups := func() map[types.NamespacedName]int {
		result := map[types.NamespacedName]int{}
		buildCleanupsSync.Range(func(k, v any) bool {
			result[k.(types.NamespacedName)] = v.(int)
			return true
		})
		return result
	}

	BeforeEach(func() {
		cfApp = &korifiv1alpha1.CFApp{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: testNamespace,
				Name:      uuid.NewString(),
			},
			Spec: korifiv1alpha1.CFAppSpec{
				DisplayName:  uuid.NewString(),
				DesiredState: "STOPPED",
				Lifecycle: korifiv1alpha1.Lifecycle{
					Type: "buildpack",
				},
			},
		}

		cfPackage = &korifiv1alpha1.CFPackage{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: testNamespace,
				Name:      uuid.NewString(),
			},
			Spec: korifiv1alpha1.CFPackageSpec{
				Type: "bits",
				AppRef: v1.LocalObjectReference{
					Name: cfApp.Name,
				},
			},
		}

		cfBuild = &korifiv1alpha1.CFBuild{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: testNamespace,
				Name:      uuid.NewString(),
			},
			Spec: korifiv1alpha1.CFBuildSpec{
				PackageRef: v1.LocalObjectReference{
					Name: cfPackage.Name,
				},
				AppRef: v1.LocalObjectReference{
					Name: cfApp.Name,
				},
				Lifecycle: korifiv1alpha1.Lifecycle{
					Type: "buildpack",
				},
			},
		}
	})

	JustBeforeEach(func() {
		Expect(adminClient.Create(ctx, cfApp)).To(Succeed())
		Expect(adminClient.Create(ctx, cfPackage)).To(Succeed())
		Expect(adminClient.Create(ctx, cfBuild)).To(Succeed())
	})

	It("sets the ObservedGeneration status field", func() {
		Eventually(func(g Gomega) {
			g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(cfBuild), cfBuild)).To(Succeed())
			g.Expect(cfBuild.Status.ObservedGeneration).To(Equal(cfBuild.Generation))
		}).Should(Succeed())
	})

	It("sets the owner reference on the build", func() {
		Eventually(func(g Gomega) {
			g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(cfBuild), cfBuild)).To(Succeed())
			g.Expect(cfBuild.OwnerReferences).To(ConsistOf(MatchFields(IgnoreExtras, Fields{
				"Kind":               Equal("CFApp"),
				"Name":               Equal(cfApp.Name),
				"Controller":         PointTo(BeTrue()),
				"BlockOwnerDeletion": PointTo(BeTrue()),
			})))
		}).Should(Succeed())
	})

	It("cleans up previous builds", func() {
		Eventually(func(g Gomega) {
			g.Expect(buildCleanups()).To(HaveKey(types.NamespacedName{
				Namespace: cfApp.Namespace,
				Name:      cfApp.Name,
			}))
		}).Should(Succeed())
	})

	It("reconciles the build", func() {
		Eventually(func(g Gomega) {
			g.Expect(reconciledBuilds()).To(HaveKey(cfBuild.Name))
		}).Should(Succeed())
	})

	Describe("package type and build type mismatch", func() {
		When("the package type is bits and build type is docker", func() {
			BeforeEach(func() {
				cfApp.Spec.Lifecycle.Type = "buildpack"
				cfPackage.Spec.Type = "bits"
				cfBuild.Spec.Lifecycle.Type = "docker"
			})

			It("fails the build", func() {
				Eventually(func(g Gomega) {
					g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(cfBuild), cfBuild)).To(Succeed())
					g.Expect(meta.IsStatusConditionFalse(cfBuild.Status.Conditions, korifiv1alpha1.SucceededConditionType)).To(BeTrue())
					g.Expect(meta.IsStatusConditionFalse(cfBuild.Status.Conditions, korifiv1alpha1.StagingConditionType)).To(BeTrue())
				}).Should(Succeed())
			})
		})

		When("the package type is docker and build type is buildpack", func() {
			BeforeEach(func() {
				cfApp.Spec.Lifecycle.Type = "docker"
				cfPackage.Spec.Type = "docker"
				cfBuild.Spec.Lifecycle.Type = "buildpack"
			})

			It("fails the build", func() {
				Eventually(func(g Gomega) {
					g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(cfBuild), cfBuild)).To(Succeed())
					g.Expect(meta.IsStatusConditionFalse(cfBuild.Status.Conditions, korifiv1alpha1.SucceededConditionType)).To(BeTrue())
					g.Expect(meta.IsStatusConditionFalse(cfBuild.Status.Conditions, korifiv1alpha1.StagingConditionType)).To(BeTrue())
				}).Should(Succeed())
			})
		})
	})

	Describe("app type and package type mismatch", func() {
		When("the app lifecycle type is buildpack and the package type is docker", func() {
			BeforeEach(func() {
				cfApp.Spec.Lifecycle.Type = "buildpack"
				cfPackage.Spec.Type = "docker"
				cfBuild.Spec.Lifecycle.Type = "docker"
			})

			It("fails the build", func() {
				Eventually(func(g Gomega) {
					g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(cfBuild), cfBuild)).To(Succeed())
					g.Expect(meta.IsStatusConditionFalse(cfBuild.Status.Conditions, korifiv1alpha1.SucceededConditionType)).To(BeTrue())
					g.Expect(meta.IsStatusConditionFalse(cfBuild.Status.Conditions, korifiv1alpha1.StagingConditionType)).To(BeTrue())
				}).Should(Succeed())
			})
		})

		When("the app lifecycle type is docker and the package type is bits", func() {
			BeforeEach(func() {
				cfApp.Spec.Lifecycle.Type = "docker"
				cfPackage.Spec.Type = "bits"
				cfBuild.Spec.Lifecycle.Type = "buildpack"
			})

			It("fails the build", func() {
				Eventually(func(g Gomega) {
					g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(cfBuild), cfBuild)).To(Succeed())
					g.Expect(meta.IsStatusConditionFalse(cfBuild.Status.Conditions, korifiv1alpha1.SucceededConditionType)).To(BeTrue())
					g.Expect(meta.IsStatusConditionFalse(cfBuild.Status.Conditions, korifiv1alpha1.StagingConditionType)).To(BeTrue())
				}).Should(Succeed())
			})
		})
	})

	When("the build succeeds", func() {
		JustBeforeEach(func() {
			Eventually(func(g Gomega) {
				g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(cfBuild), cfBuild)).To(Succeed())
				delegateInvokedCondition := meta.FindStatusCondition(cfBuild.Status.Conditions, "delegateInvokedCondition")
				g.Expect(delegateInvokedCondition).NotTo(BeNil())
			}).Should(Succeed())

			Expect(k8s.Patch(ctx, adminClient, cfBuild, func() {
				meta.SetStatusCondition(&cfBuild.Status.Conditions, metav1.Condition{
					Type:               korifiv1alpha1.SucceededConditionType,
					Status:             metav1.ConditionTrue,
					LastTransitionTime: metav1.Now(),
					Reason:             "ok",
					Message:            "ok",
				})
			})).To(Succeed())
		})

		It("stops reconciling", func() {
			reoncileCount := reconciledBuilds()[cfBuild.Name]
			Consistently(func(g Gomega) {
				g.Expect(reconciledBuilds()[cfBuild.Name]).To(Equal(reoncileCount))
			}).Should(Succeed())
		})
	})
})
