package integration_test

import (
	"context"
	"time"

	workloadsv1alpha1 "code.cloudfoundry.org/korifi/controllers/apis/workloads/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/controllers/workloads"
	. "code.cloudfoundry.org/korifi/controllers/controllers/workloads/testutils"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	hncv1alpha2 "sigs.k8s.io/hierarchical-namespaces/api/v1alpha2"
)

var _ = Describe("CFOrgReconciler Integration Tests", func() {
	const (
		orgName = "my-org"
	)
	var (
		testCtx       context.Context
		rootNamespace *v1.Namespace
		orgGUID       string
		cfOrg         workloadsv1alpha1.CFOrg
	)

	BeforeEach(func() {
		testCtx = context.Background()
		rootNamespace = createNamespace(testCtx, k8sClient, PrefixedGUID("root-ns"))
		orgGUID = PrefixedGUID("cf-org")
		cfOrg = workloadsv1alpha1.CFOrg{
			ObjectMeta: metav1.ObjectMeta{
				Name:      orgGUID,
				Namespace: rootNamespace.Name,
			},
			Spec: workloadsv1alpha1.CFOrgSpec{
				Name: orgName,
			},
		}
	})

	AfterEach(func() {
		Expect(k8sClient.Delete(context.Background(), rootNamespace)).To(Succeed())
	})

	When("the CFOrg is created", func() {
		BeforeEach(func() {
			createNamespace(testCtx, k8sClient, orgGUID)
			Expect(k8sClient.Create(testCtx, &hncv1alpha2.HierarchyConfiguration{
				ObjectMeta: metav1.ObjectMeta{Namespace: orgGUID, Name: "hierarchy"},
			})).To(Succeed())
		})

		JustBeforeEach(func() {
			Expect(k8sClient.Create(testCtx, &cfOrg)).To(Succeed())
		})

		It("creates a subnamespace anchor in the root namespace", func() {
			Eventually(func(g Gomega) {
				var anchor hncv1alpha2.SubnamespaceAnchor
				g.Expect(k8sClient.Get(testCtx, types.NamespacedName{Namespace: rootNamespace.Name, Name: orgGUID}, &anchor)).To(Succeed())
				g.Expect(anchor.Labels).To(HaveKeyWithValue(workloads.OrgNameLabel, orgName))
			}).Should(Succeed())
		})

		It("makes the CFOrg the owner of its subnamespace anchor", func() {
			Eventually(func(g Gomega) {
				var anchor hncv1alpha2.SubnamespaceAnchor
				g.Expect(k8sClient.Get(testCtx, types.NamespacedName{Namespace: rootNamespace.Name, Name: orgGUID}, &anchor)).To(Succeed())
				g.Expect(anchor.ObjectMeta.OwnerReferences).To(ConsistOf([]metav1.OwnerReference{
					{
						APIVersion: "workloads.cloudfoundry.org/v1alpha1",
						Kind:       "CFOrg",
						Name:       cfOrg.Name,
						UID:        cfOrg.GetUID(),
					},
				}))
			}).Should(Succeed())
		})

		When("the subnamespace anchor status changes to ok", func() {
			JustBeforeEach(func() {
				var subspaceAnchor hncv1alpha2.SubnamespaceAnchor
				Eventually(func() error {
					return k8sClient.Get(testCtx, types.NamespacedName{Namespace: rootNamespace.Name, Name: orgGUID}, &subspaceAnchor)
				}).Should(Succeed())
				subspaceAnchor.Status.State = hncv1alpha2.Ok
				Expect(k8sClient.Update(testCtx, &subspaceAnchor)).To(Succeed())
			})

			It("sets the AllowCascadingDeletion property on the HNC hierarchyconfiguration in the namespace", func() {
				Eventually(func(g Gomega) {
					var hc hncv1alpha2.HierarchyConfiguration
					err := k8sClient.Get(testCtx, types.NamespacedName{Namespace: orgGUID, Name: "hierarchy"}, &hc)
					g.Expect(err).To(BeNil())
					g.Expect(hc.Spec.AllowCascadingDeletion).To(BeTrue())
				}).Should(Succeed())
			})

			It("sets the CFOrg 'Ready' condition to 'True'", func() {
				Eventually(func(g Gomega) {
					var latestOrg workloadsv1alpha1.CFOrg
					err := k8sClient.Get(testCtx, types.NamespacedName{Namespace: rootNamespace.Name, Name: orgGUID}, &latestOrg)
					g.Expect(err).To(BeNil())
					g.Expect(meta.IsStatusConditionTrue(latestOrg.Status.Conditions, "Ready")).To(BeTrue())
				}, 5*time.Second).Should(Succeed())
			})

			It("sets the CFOrg status GUID", func() {
				Eventually(func(g Gomega) {
					var latestOrg workloadsv1alpha1.CFOrg
					err := k8sClient.Get(testCtx, types.NamespacedName{Namespace: rootNamespace.Name, Name: orgGUID}, &latestOrg)
					g.Expect(err).To(BeNil())
					g.Expect(latestOrg.Status.GUID).To(Equal(cfOrg.ObjectMeta.Name))
				}, 5*time.Second).Should(Succeed())
			})
		})

		When("the subnamespace anchor never becomes ready", func() {
			It("leaves the CFOrg 'Ready' condition as 'Unknown'", func() {
				Eventually(func(g Gomega) {
					var latestOrg workloadsv1alpha1.CFOrg
					err := k8sClient.Get(testCtx, types.NamespacedName{Namespace: rootNamespace.Name, Name: orgGUID}, &latestOrg)
					g.Expect(err).To(BeNil())
					g.Expect(meta.IsStatusConditionPresentAndEqual(latestOrg.Status.Conditions, "Ready", metav1.ConditionUnknown)).To(BeTrue())
				}).Should(Succeed())

				Consistently(func(g Gomega) {
					var latestOrg workloadsv1alpha1.CFOrg
					err := k8sClient.Get(testCtx, types.NamespacedName{Namespace: rootNamespace.Name, Name: orgGUID}, &latestOrg)
					g.Expect(err).To(BeNil())
					g.Expect(meta.IsStatusConditionPresentAndEqual(latestOrg.Status.Conditions, "Ready", metav1.ConditionUnknown)).To(BeTrue())
				}).Should(Succeed())
			})
		})
	})
})
