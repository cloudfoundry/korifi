package workloads_test

import (
	"context"

	"code.cloudfoundry.org/korifi/tools"

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
			cfPackage = BuildCFPackageCRObject(cfPackageGUID, cfSpace.Status.GUID, cfAppGUID)
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
})
