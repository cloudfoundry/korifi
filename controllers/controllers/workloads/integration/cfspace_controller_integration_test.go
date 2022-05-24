package integration_test

import (
	"context"
	"time"

	"code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/controllers/workloads"
	. "code.cloudfoundry.org/korifi/controllers/controllers/workloads/testutils"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	hncv1alpha2 "sigs.k8s.io/hierarchical-namespaces/api/v1alpha2"
)

var _ = Describe("CFSpace Reconciler Integration Tests", func() {
	const (
		spaceName = "my-space"
	)

	var (
		ctx          context.Context
		orgNamespace *corev1.Namespace
		spaceGUID    string
		cfSpace      *v1alpha1.CFSpace
	)

	BeforeEach(func() {
		ctx = context.Background()
		orgNamespace = createNamespaceWithCleanup(ctx, k8sClient, PrefixedGUID("cf-org"))
		spaceGUID = PrefixedGUID("cf-space")
		cfSpace = &v1alpha1.CFSpace{
			ObjectMeta: metav1.ObjectMeta{
				Name:      spaceGUID,
				Namespace: orgNamespace.Name,
			},
			Spec: v1alpha1.CFSpaceSpec{
				DisplayName: spaceName,
			},
		}
	})

	When("the CFSpace is created", func() {
		JustBeforeEach(func() {
			Expect(k8sClient.Create(ctx, cfSpace)).To(Succeed())
		})

		It("creates a subnamespace anchor in the CFOrg namespace", func() {
			Eventually(func(g Gomega) {
				var anchor hncv1alpha2.SubnamespaceAnchor
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: orgNamespace.Name, Name: spaceGUID}, &anchor)).To(Succeed())
				g.Expect(anchor.Labels).To(HaveKeyWithValue(workloads.SpaceNameLabel, spaceName))
			}).Should(Succeed())
		})

		It("sets the Owner Reference to CFSpace on the subnamespace anchor", func() {
			Eventually(func(g Gomega) {
				var anchor hncv1alpha2.SubnamespaceAnchor
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: orgNamespace.Name, Name: spaceGUID}, &anchor)).To(Succeed())
				g.Expect(anchor.ObjectMeta.OwnerReferences).To(ConsistOf([]metav1.OwnerReference{
					{
						APIVersion: "korifi.cloudfoundry.org/v1alpha1",
						Kind:       "CFSpace",
						Name:       cfSpace.Name,
						UID:        cfSpace.GetUID(),
					},
				}))
			}).Should(Succeed())
		})

		It("sets the status condition \"Ready\" to \"Unknown\"", func() {
			Eventually(func(g Gomega) {
				var createdCFSpace v1alpha1.CFSpace
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: orgNamespace.Name, Name: spaceGUID}, &createdCFSpace)).To(Succeed())
				g.Expect(meta.IsStatusConditionPresentAndEqual(createdCFSpace.Status.Conditions, "Ready", metav1.ConditionUnknown)).To(BeTrue())
			}, 5*time.Second).Should(Succeed())
		})

		When("the subnamespace anchor status changes to ok and the namespace has been created", func() {
			JustBeforeEach(func() {
				subnamespaceAnchor := new(hncv1alpha2.SubnamespaceAnchor)
				Eventually(func() error {
					return k8sClient.Get(ctx, types.NamespacedName{Namespace: orgNamespace.Name, Name: spaceGUID}, subnamespaceAnchor)
				}).Should(Succeed())
				subnamespaceAnchor.Status.State = hncv1alpha2.Ok
				Expect(k8sClient.Update(ctx, subnamespaceAnchor)).To(Succeed())

				createNamespaceWithCleanup(ctx, k8sClient, spaceGUID)
			})

			It("creates the kpack service account", func() {
				var serviceAccount corev1.ServiceAccount
				Eventually(func() error {
					return k8sClient.Get(ctx, types.NamespacedName{Namespace: spaceGUID, Name: "kpack-service-account"}, &serviceAccount)
				}).Should(Succeed())

				Expect(serviceAccount.ImagePullSecrets).To(Equal([]corev1.LocalObjectReference{
					{Name: packageRegistrySecretName},
				}))
				Expect(serviceAccount.Secrets).To(Equal([]corev1.ObjectReference{
					{Name: packageRegistrySecretName},
				}))
			})

			It("creates the eirini service account", func() {
				Eventually(func() error {
					var serviceAccount corev1.ServiceAccount
					return k8sClient.Get(ctx, types.NamespacedName{Namespace: spaceGUID, Name: "eirini"}, &serviceAccount)
				}).Should(Succeed())
			})

			It("sets the CFSpace 'Ready' condition to 'True'", func() {
				Eventually(func(g Gomega) {
					var createdCFSpace v1alpha1.CFSpace
					g.Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: orgNamespace.Name, Name: spaceGUID}, &createdCFSpace)).To(Succeed())
					g.Expect(meta.IsStatusConditionTrue(createdCFSpace.Status.Conditions, "Ready")).To(BeTrue())
				}, 5*time.Second).Should(Succeed())
			})

			It("sets the CFSpace status GUID", func() {
				Eventually(func(g Gomega) {
					var createdCFSpace v1alpha1.CFSpace
					g.Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: orgNamespace.Name, Name: spaceGUID}, &createdCFSpace)).To(Succeed())
					g.Expect(createdCFSpace.Status.GUID).To(Equal(spaceGUID))
				}, 5*time.Second).Should(Succeed())
			})
		})

		When("the subnamespace anchor never becomes ready", func() {
			It("leaves the CFSpace 'Ready' condition as 'Unknown'", func() {
				Eventually(func(g Gomega) {
					var createdCFSpace v1alpha1.CFSpace
					err := k8sClient.Get(ctx, types.NamespacedName{Namespace: orgNamespace.Name, Name: spaceGUID}, &createdCFSpace)
					g.Expect(err).To(BeNil())
					g.Expect(meta.IsStatusConditionPresentAndEqual(createdCFSpace.Status.Conditions, "Ready", metav1.ConditionUnknown)).To(BeTrue())
				}).Should(Succeed())

				Consistently(func(g Gomega) {
					var createdCFSpace v1alpha1.CFSpace
					err := k8sClient.Get(ctx, types.NamespacedName{Namespace: orgNamespace.Name, Name: spaceGUID}, &createdCFSpace)
					g.Expect(err).To(BeNil())
					g.Expect(meta.IsStatusConditionPresentAndEqual(createdCFSpace.Status.Conditions, "Ready", metav1.ConditionUnknown)).To(BeTrue())
				}).Should(Succeed())
			})
		})
	})
})
