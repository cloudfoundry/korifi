package packages_test

import (
	"context"
	"errors"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/controllers/workloads/packages"
	"code.cloudfoundry.org/korifi/tools"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("CFPackageReconciler Integration Tests", func() {
	var (
		cfApp     *korifiv1alpha1.CFApp
		cfPackage *korifiv1alpha1.CFPackage
	)

	BeforeEach(func() {
		cfApp = &korifiv1alpha1.CFApp{
			ObjectMeta: metav1.ObjectMeta{
				Name:      uuid.NewString(),
				Namespace: testNamespace,
			},
			Spec: korifiv1alpha1.CFAppSpec{
				DisplayName:  "test-app-name",
				DesiredState: "STOPPED",
				Lifecycle: korifiv1alpha1.Lifecycle{
					Type: "buildpack",
				},
			},
		}
		Expect(adminClient.Create(context.Background(), cfApp)).To(Succeed())

		cfPackage = &korifiv1alpha1.CFPackage{
			ObjectMeta: metav1.ObjectMeta{
				Name:      uuid.NewString(),
				Namespace: testNamespace,
				Finalizers: []string{
					korifiv1alpha1.CFPackageFinalizerName,
				},
			},
			Spec: korifiv1alpha1.CFPackageSpec{
				Type: "bits",
				AppRef: corev1.LocalObjectReference{
					Name: cfApp.Name,
				},
				Source: korifiv1alpha1.PackageSource{
					Registry: korifiv1alpha1.Registry{
						Image:            "hello",
						ImagePullSecrets: []corev1.LocalObjectReference{{Name: "source-registry-image-pull-secret"}},
					},
				},
			},
		}
	})

	JustBeforeEach(func() {
		Expect(adminClient.Create(context.Background(), cfPackage)).To(Succeed())
	})

	When("a new CFPackage resource is created", func() {
		var cleanCallCount int

		BeforeEach(func() {
			cleanCallCount = packageCleaner.CleanCallCount()
		})

		It("initializes it", func() {
			Eventually(func(g Gomega) {
				g.Expect(adminClient.Get(context.Background(), client.ObjectKeyFromObject(cfPackage), cfPackage)).To(Succeed())

				initializedCondition := meta.FindStatusCondition(cfPackage.Status.Conditions, packages.InitializedConditionType)
				g.Expect(initializedCondition).NotTo(BeNil())
				g.Expect(initializedCondition.ObservedGeneration).To(Equal(cfPackage.Generation))

				g.Expect(cfPackage.GetOwnerReferences()).To(ConsistOf(metav1.OwnerReference{
					APIVersion:         korifiv1alpha1.GroupVersion.Identifier(),
					Kind:               "CFApp",
					Name:               cfApp.Name,
					UID:                cfApp.UID,
					Controller:         tools.PtrTo(true),
					BlockOwnerDeletion: tools.PtrTo(true),
				}))
			}).Should(Succeed())
		})

		It("sets the Ready condition to true", func() {
			Eventually(func(g Gomega) {
				g.Expect(adminClient.Get(context.Background(), client.ObjectKeyFromObject(cfPackage), cfPackage)).To(Succeed())
				g.Expect(meta.IsStatusConditionTrue(cfPackage.Status.Conditions, korifiv1alpha1.StatusConditionReady)).To(BeTrue())
			}).Should(Succeed())
		})

		It("deletes the older packages for the same app", func() {
			Eventually(func(g Gomega) {
				g.Expect(packageCleaner.CleanCallCount()).To(BeNumerically(">", cleanCallCount))

				var cleanedApps []types.NamespacedName
				for currCall := cleanCallCount; currCall < packageCleaner.CleanCallCount(); currCall++ {
					cleanedApps = append(cleanedApps, types.NamespacedName{Namespace: testNamespace, Name: cfApp.Name})
				}
				g.Expect(cleanedApps).To(ContainElement(types.NamespacedName{Namespace: cfApp.Namespace, Name: cfApp.Name}))
			}).Should(Succeed())
		})

		It("sets the ObservedGeneration status field", func() {
			Eventually(func(g Gomega) {
				g.Expect(adminClient.Get(context.Background(), client.ObjectKeyFromObject(cfPackage), cfPackage)).To(Succeed())
				g.Expect(cfPackage.Status.ObservedGeneration).To(Equal(cfPackage.Generation))
			}).Should(Succeed())
		})

		When("the package does not have source", func() {
			BeforeEach(func() {
				cfPackage.Spec.Source = korifiv1alpha1.PackageSource{}
			})

			It("sets the ready condition to false", func() {
				Eventually(func(g Gomega) {
					g.Expect(adminClient.Get(context.Background(), client.ObjectKeyFromObject(cfPackage), cfPackage)).To(Succeed())
					g.Expect(meta.IsStatusConditionTrue(cfPackage.Status.Conditions, packages.InitializedConditionType)).To(BeTrue())
					g.Expect(meta.IsStatusConditionFalse(cfPackage.Status.Conditions, korifiv1alpha1.StatusConditionReady)).To(BeTrue())
				}).Should(Succeed())
			})
		})
	})

	Describe("finalization", func() {
		var deleteCount int

		BeforeEach(func() {
			deleteCount = imageDeleter.DeleteCallCount()
		})

		JustBeforeEach(func() {
			Eventually(func(g Gomega) {
				g.Expect(adminClient.Get(context.Background(), client.ObjectKeyFromObject(cfPackage), cfPackage)).To(Succeed())
				g.Expect(meta.IsStatusConditionTrue(cfPackage.Status.Conditions, packages.InitializedConditionType)).To(BeTrue())
			}).Should(Succeed())

			Expect(adminClient.Delete(context.Background(), cfPackage)).To(Succeed())
		})

		It("deletes itself and the corresponding source image", func() {
			Eventually(func(g Gomega) {
				g.Expect(imageDeleter.DeleteCallCount()).To(BeNumerically(">", deleteCount))

				_, creds, ref, tagsToDelete := imageDeleter.DeleteArgsForCall(deleteCount)
				g.Expect(creds.Namespace).To(Equal(testNamespace))
				g.Expect(creds.SecretNames).To(ConsistOf("package-repo-secret-name"))
				g.Expect(ref).To(Equal("hello"))
				g.Expect(tagsToDelete).To(ConsistOf(cfPackage.Name))
			}).Should(Succeed())

			Eventually(func(g Gomega) {
				g.Expect(adminClient.Get(context.Background(), client.ObjectKeyFromObject(cfPackage), cfPackage)).To(MatchError(ContainSubstring("not found")))
			}).Should(Succeed())
		})

		When("the package type is docker", func() {
			BeforeEach(func() {
				cfPackage.Spec.Type = "docker"
			})

			It("does not delete the image", func() {
				Consistently(func(g Gomega) {
					g.Expect(imageDeleter.DeleteCallCount()).To(Equal(deleteCount))
				}).Should(Succeed())
			})
		})

		When("the package doesn't have an image set", func() {
			BeforeEach(func() {
				cfPackage.Spec.Source.Registry.Image = ""
			})

			It("doesn't try to delete any image", func() {
				Consistently(func(g Gomega) {
					g.Expect(imageDeleter.DeleteCallCount()).To(Equal(deleteCount))
				}).Should(Succeed())
			})
		})

		When("deletion fails", func() {
			BeforeEach(func() {
				imageDeleter.DeleteReturns(errors.New("oops"))
			})

			It("ignores the errors and finishes finalization", func() {
				Eventually(func(g Gomega) {
					g.Expect(adminClient.Get(context.Background(), client.ObjectKeyFromObject(cfPackage), cfPackage)).To(MatchError(ContainSubstring("not found")))
				}).Should(Succeed())
			})
		})
	})
})
