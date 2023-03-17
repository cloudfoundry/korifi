package workloads_test

import (
	"context"
	"errors"

	"code.cloudfoundry.org/korifi/tools"
	"code.cloudfoundry.org/korifi/tools/k8s"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/controllers/workloads"
	. "code.cloudfoundry.org/korifi/controllers/controllers/workloads/testutils"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
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
		BeforeEach(func() {
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

			Expect(meta.IsStatusConditionFalse(createdCFPackage.Status.Conditions, workloads.StatusConditionReady)).To(BeTrue())
			Expect(meta.FindStatusCondition(createdCFPackage.Status.Conditions, workloads.StatusConditionReady).ObservedGeneration).To(Equal(createdCFPackage.Generation))

			Expect(createdCFPackage.GetOwnerReferences()).To(ConsistOf(metav1.OwnerReference{
				APIVersion:         korifiv1alpha1.GroupVersion.Identifier(),
				Kind:               "CFApp",
				Name:               cfApp.Name,
				UID:                cfApp.UID,
				Controller:         tools.PtrTo(true),
				BlockOwnerDeletion: tools.PtrTo(true),
			}))
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
					g.Expect(meta.IsStatusConditionTrue(createdCFPackage.Status.Conditions, workloads.StatusConditionReady)).To(BeTrue())
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

			_, creds, ref := imageDeleter.DeleteArgsForCall(deleteCount)
			Expect(creds.Namespace).To(Equal(cfSpace.Status.GUID))
			Expect(creds.SecretName).To(Equal("package-repo-secret-name"))
			Expect(ref).To(Equal(imageRef))

			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(cfPackage), cfPackage)).To(MatchError(ContainSubstring("not found")))
			}).Should(Succeed())
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
