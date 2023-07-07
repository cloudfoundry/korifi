package workloads_test

import (
	"context"
	"errors"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/controllers/shared"
	"code.cloudfoundry.org/korifi/controllers/controllers/workloads"
	. "code.cloudfoundry.org/korifi/controllers/controllers/workloads/testutils"
	"code.cloudfoundry.org/korifi/tools"
	"code.cloudfoundry.org/korifi/tools/k8s"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
)

var _ = Describe("CFPackageReconciler Integration Tests", func() {
	var (
		cfSpace       *korifiv1alpha1.CFSpace
		cfApp         *korifiv1alpha1.CFApp
		cfAppGUID     string
		cfPackage     *korifiv1alpha1.CFPackage
		cfPackageGUID string
	)

	BeforeEach(func() {
		cfSpace = createSpace(cfOrg)
		cfAppGUID = GenerateGUID()
		cfPackageGUID = GenerateGUID()

		cfApp = BuildCFAppCRObject(cfAppGUID, cfSpace.Status.GUID)

		Expect(k8sClient.Create(context.Background(), cfApp)).To(Succeed())
	})

	When("a new CFPackage resource is created", func() {
		var cleanCallCount int

		BeforeEach(func() {
			cleanCallCount = packageCleaner.CleanCallCount()

			cfPackage = BuildCFPackageCRObject(cfPackageGUID, cfSpace.Status.GUID, cfAppGUID, "ref")
			cfPackage.Spec.Source = korifiv1alpha1.PackageSource{}
			Expect(k8sClient.Create(context.Background(), cfPackage)).To(Succeed())
		})

		It("initializes it", func() {
			var createdCFPackage korifiv1alpha1.CFPackage
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(cfPackage), &createdCFPackage)).To(Succeed())
				g.Expect(meta.IsStatusConditionTrue(createdCFPackage.Status.Conditions, workloads.InitializedConditionType)).To(BeTrue())
			}).Should(Succeed())

			Expect(meta.FindStatusCondition(createdCFPackage.Status.Conditions, workloads.InitializedConditionType).ObservedGeneration).To(Equal(createdCFPackage.Generation))

			Expect(meta.IsStatusConditionFalse(createdCFPackage.Status.Conditions, shared.StatusConditionReady)).To(BeTrue())
			Expect(meta.FindStatusCondition(createdCFPackage.Status.Conditions, shared.StatusConditionReady).ObservedGeneration).To(Equal(createdCFPackage.Generation))

			Expect(createdCFPackage.GetOwnerReferences()).To(ConsistOf(metav1.OwnerReference{
				APIVersion:         korifiv1alpha1.GroupVersion.Identifier(),
				Kind:               "CFApp",
				Name:               cfApp.Name,
				UID:                cfApp.UID,
				Controller:         tools.PtrTo(true),
				BlockOwnerDeletion: tools.PtrTo(true),
			}))
		})

		It("deletes the older packages for the same app", func() {
			Eventually(func(g Gomega) {
				g.Expect(packageCleaner.CleanCallCount()).To(BeNumerically(">", cleanCallCount))
			}).Should(Succeed())

			_, app := packageCleaner.CleanArgsForCall(cleanCallCount)
			Expect(app.Name).To(Equal(cfAppGUID))
			Expect(app.Namespace).To(Equal(cfSpace.Status.GUID))
		})

		It("sets the ObservedGeneration status field", func() {
			var createdCFPackage korifiv1alpha1.CFPackage
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(cfPackage), &createdCFPackage)).To(Succeed())

				g.Expect(createdCFPackage.Status.ObservedGeneration).To(Equal(createdCFPackage.Generation))
			}).Should(Succeed())
		})

		When("the package is updated with its source image", func() {
			var createdCFPackage korifiv1alpha1.CFPackage

			BeforeEach(func() {
				Eventually(func(g Gomega) {
					g.Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(cfPackage), &createdCFPackage)).To(Succeed())
					g.Expect(meta.IsStatusConditionTrue(createdCFPackage.Status.Conditions, workloads.InitializedConditionType)).To(BeTrue())
				}).Should(Succeed())
			})

			JustBeforeEach(func() {
				Expect(k8s.PatchResource(ctx, k8sClient, &createdCFPackage, func() {
					createdCFPackage.Spec.Source.Registry.Image = "hello"
				})).To(Succeed())
			})

			It("sets the ready condition to true", func() {
				Eventually(func(g Gomega) {
					g.Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(cfPackage), &createdCFPackage)).To(Succeed())
					g.Expect(meta.IsStatusConditionTrue(createdCFPackage.Status.Conditions, shared.StatusConditionReady)).To(BeTrue())
				}).Should(Succeed())
			})
		})
	})

	When("a CFPackage is deleted", func() {
		var (
			deleteCount int
			imageRef    string
		)

		BeforeEach(func() {
			imageRef = GenerateGUID()
			cfPackage = BuildCFPackageCRObject(cfPackageGUID, cfSpace.Status.GUID, cfAppGUID, imageRef)
			deleteCount = imageDeleter.DeleteCallCount()
		})

		JustBeforeEach(func() {
			Expect(k8sClient.Create(context.Background(), cfPackage)).To(Succeed())

			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(cfPackage), cfPackage)).To(Succeed())
				g.Expect(meta.IsStatusConditionTrue(cfPackage.Status.Conditions, workloads.InitializedConditionType)).To(BeTrue())
			}).Should(Succeed())

			Expect(k8sClient.Delete(context.Background(), cfPackage)).To(Succeed())
		})

		It("deletes itself and the corresponding source image", func() {
			Eventually(func(g Gomega) {
				g.Expect(imageDeleter.DeleteCallCount()).To(Equal(deleteCount + 1))
			}).Should(Succeed())

			_, creds, ref, tagsToDelete := imageDeleter.DeleteArgsForCall(deleteCount)
			Expect(creds.Namespace).To(Equal(cfSpace.Status.GUID))
			Expect(creds.SecretNames).To(ConsistOf("package-repo-secret-name"))
			Expect(ref).To(Equal(imageRef))
			Expect(tagsToDelete).To(ConsistOf(cfPackage.Name))

			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(cfPackage), cfPackage)).To(MatchError(ContainSubstring("not found")))
			}).Should(Succeed())
		})

		It("writes a log message", func() {
			Eventually(logOutput).Should(gbytes.Say("finalizer removed"))
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
					g.Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(cfPackage), cfPackage)).To(MatchError(ContainSubstring("not found")))
				}).Should(Succeed())
			})
		})
	})
})
