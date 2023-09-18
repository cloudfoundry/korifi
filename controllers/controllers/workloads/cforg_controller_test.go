package workloads_test

import (
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	. "code.cloudfoundry.org/korifi/controllers/controllers/workloads/testutils"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/pod-security-admission/api"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("CFOrgReconciler Integration Tests", func() {
	var (
		orgGUID string
		cfOrg   *korifiv1alpha1.CFOrg
	)

	BeforeEach(func() {
		orgGUID = PrefixedGUID("cf-org")
		cfOrg = &korifiv1alpha1.CFOrg{
			ObjectMeta: metav1.ObjectMeta{
				Name:      orgGUID,
				Namespace: cfRootNamespace,
			},
			Spec: korifiv1alpha1.CFOrgSpec{
				DisplayName: uuid.NewString(),
			},
		}
		Expect(adminClient.Create(ctx, cfOrg)).To(Succeed())
	})

	It("creates an org namespace and sets labels", func() {
		Eventually(func(g Gomega) {
			var orgNamespace corev1.Namespace
			g.Expect(adminClient.Get(ctx, types.NamespacedName{Name: orgGUID}, &orgNamespace)).To(Succeed())

			g.Expect(orgNamespace.Labels).To(SatisfyAll(
				HaveKeyWithValue(korifiv1alpha1.OrgNameKey, korifiv1alpha1.OrgSpaceDeprecatedName),
				HaveKeyWithValue(korifiv1alpha1.OrgGUIDKey, orgGUID),
				HaveKeyWithValue(api.EnforceLevelLabel, string(api.LevelRestricted)),
			))
			g.Expect(orgNamespace.Annotations).To(HaveKeyWithValue(korifiv1alpha1.OrgNameKey, cfOrg.Spec.DisplayName))
		}).Should(Succeed())
	})

	It("sets the finalizer on cfOrg", func() {
		Eventually(func(g Gomega) {
			g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(cfOrg), cfOrg)).To(Succeed())
			g.Expect(cfOrg.ObjectMeta.Finalizers).To(ConsistOf("cfOrg.korifi.cloudfoundry.org"))
		}).Should(Succeed())
	})

	It("propagates the image-registry-credentials secrets from root-ns to org namespace", func() {
		Eventually(func(g Gomega) {
			g.Expect(adminClient.Get(ctx, types.NamespacedName{Namespace: cfOrg.Name, Name: imageRegistrySecret.Name}, &corev1.Secret{})).To(Succeed())
		}).Should(Succeed())
	})

	It("sets the ready status on the CFOrg", func() {
		Eventually(func(g Gomega) {
			var createdOrg korifiv1alpha1.CFOrg
			g.Expect(adminClient.Get(ctx, types.NamespacedName{Namespace: cfRootNamespace, Name: orgGUID}, &createdOrg)).To(Succeed())

			g.Expect(createdOrg.Status.GUID).To(Equal(orgGUID))
			g.Expect(createdOrg.Status.ObservedGeneration).To(Equal(createdOrg.Generation))
			g.Expect(meta.IsStatusConditionTrue(createdOrg.Status.Conditions, "Ready")).To(BeTrue())
		}).Should(Succeed())
	})

	It("sets restricted pod security labels on the namespace", func() {
		Eventually(func(g Gomega) {
			var ns corev1.Namespace
			g.Expect(adminClient.Get(ctx, types.NamespacedName{Name: orgGUID}, &ns)).To(Succeed())
			g.Expect(ns.Labels).To(HaveKeyWithValue(api.EnforceLevelLabel, string(api.LevelRestricted)))
		}).Should(Succeed())
	})
})
