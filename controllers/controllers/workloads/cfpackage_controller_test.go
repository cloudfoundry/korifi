package workloads_test

import (
	"context"
	"errors"

	"code.cloudfoundry.org/korifi/tools"
	"sigs.k8s.io/controller-runtime/pkg/client"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
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
			Expect(k8sClient.Create(context.Background(), cfPackage)).To(Succeed())
		})

		It("eventually reconciles to set the owner reference on the CFPackage", func() {
			Eventually(func() []metav1.OwnerReference {
				var createdCFPackage korifiv1alpha1.CFPackage
				err := k8sClient.Get(context.Background(), types.NamespacedName{Name: cfPackageGUID, Namespace: cfSpace.Status.GUID}, &createdCFPackage)
				if err != nil {
					return nil
				}
				return createdCFPackage.GetOwnerReferences()
			}).Should(ConsistOf(metav1.OwnerReference{
				APIVersion:         korifiv1alpha1.GroupVersion.Identifier(),
				Kind:               "CFApp",
				Name:               cfApp.Name,
				UID:                cfApp.UID,
				Controller:         tools.PtrTo(true),
				BlockOwnerDeletion: tools.PtrTo(true),
			}))
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

			// wait for package to have reconciled at least once
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(cfPackage), cfPackage)).To(Succeed())
				g.Expect(cfPackage.Finalizers).ToNot(BeEmpty())
			}).Should(Succeed())

			Expect(k8sClient.Delete(context.Background(), cfPackage)).To(Succeed())
		})

		It("deletes itself and the corresponding source image", func() {
			Eventually(func(g Gomega) {
				g.Expect(imageDeleter.DeleteCallCount()).To(Equal(deleteCount + 1))
			}).Should(Succeed())

			_, ref := imageDeleter.DeleteArgsForCall(deleteCount)
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
