package orgs_test

import (
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"

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
	var cfOrg *korifiv1alpha1.CFOrg

	BeforeEach(func() {
		cfOrg = &korifiv1alpha1.CFOrg{
			ObjectMeta: metav1.ObjectMeta{
				Name:      uuid.NewString(),
				Namespace: testNamespace,
				Finalizers: []string{
					korifiv1alpha1.CFOrgFinalizerName,
				},
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
			g.Expect(adminClient.Get(ctx, types.NamespacedName{Name: cfOrg.Name}, &orgNamespace)).To(Succeed())

			g.Expect(orgNamespace.Labels).To(SatisfyAll(
				HaveKeyWithValue(korifiv1alpha1.OrgNameKey, korifiv1alpha1.OrgSpaceDeprecatedName),
				HaveKeyWithValue(korifiv1alpha1.OrgGUIDKey, cfOrg.Name),
				HaveKeyWithValue(api.EnforceLevelLabel, string(api.LevelRestricted)),
			))
			g.Expect(orgNamespace.Annotations).To(HaveKeyWithValue(korifiv1alpha1.OrgNameKey, cfOrg.Spec.DisplayName))
		}).Should(Succeed())
	})

	It("propagates the image-registry-credentials secrets from root-ns to org namespace", func() {
		Eventually(func(g Gomega) {
			g.Expect(adminClient.Get(ctx, types.NamespacedName{Namespace: cfOrg.Name, Name: packageRegistrySecretName}, &corev1.Secret{})).To(Succeed())
		}).Should(Succeed())
	})

	It("sets the ready status on the CFOrg", func() {
		Eventually(func(g Gomega) {
			g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(cfOrg), cfOrg)).To(Succeed())

			g.Expect(cfOrg.Status.GUID).To(Equal(cfOrg.Name))
			g.Expect(cfOrg.Status.ObservedGeneration).To(Equal(cfOrg.Generation))
			g.Expect(meta.IsStatusConditionTrue(cfOrg.Status.Conditions, korifiv1alpha1.StatusConditionReady)).To(BeTrue())
		}).Should(Succeed())
	})
})
