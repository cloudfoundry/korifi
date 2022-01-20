package integration_test

import (
	"context"
	"time"

	workloadsv1alpha1 "code.cloudfoundry.org/cf-k8s-controllers/controllers/apis/workloads/v1alpha1"
	. "code.cloudfoundry.org/cf-k8s-controllers/controllers/controllers/workloads/testutils"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
)

var _ = Describe("CFPackageReconciler", func() {
	var (
		namespaceGUID string
		ns            *corev1.Namespace
		cfApp         *workloadsv1alpha1.CFApp
		cfAppGUID     string
		cfPackage     *workloadsv1alpha1.CFPackage
		cfPackageGUID string
	)

	BeforeEach(func() {
		namespaceGUID = GenerateGUID()
		cfAppGUID = GenerateGUID()
		cfPackageGUID = GenerateGUID()
		ns = createNamespace(context.Background(), k8sClient, namespaceGUID)
		DeferCleanup(func() {
			_ = k8sClient.Delete(context.Background(), ns)
		})

		cfApp = BuildCFAppCRObject(cfAppGUID, namespaceGUID)
		Expect(k8sClient.Create(context.Background(), cfApp)).To(Succeed())
	})

	When("a new CFPackage resource is created", func() {
		BeforeEach(func() {
			cfPackage = BuildCFPackageCRObject(cfPackageGUID, namespaceGUID, cfAppGUID)
			Expect(k8sClient.Create(context.Background(), cfPackage)).To(Succeed())
		})

		It("eventually reconciles to set the owner reference on the CFPackage", func() {
			Eventually(func() []metav1.OwnerReference {
				var createdCFPackage workloadsv1alpha1.CFPackage
				err := k8sClient.Get(context.Background(), types.NamespacedName{Name: cfPackageGUID, Namespace: namespaceGUID}, &createdCFPackage)
				if err != nil {
					return nil
				}
				return createdCFPackage.GetOwnerReferences()
			}, 5*time.Second).Should(ConsistOf(metav1.OwnerReference{
				APIVersion: workloadsv1alpha1.GroupVersion.Identifier(),
				Kind:       "CFApp",
				Name:       cfApp.Name,
				UID:        cfApp.UID,
			}))
		})
	})
})
