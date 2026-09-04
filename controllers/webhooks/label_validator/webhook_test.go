package label_validator_test

import (
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/tools/k8s"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("LabelValidatorWebhook", func() {
	var org *korifiv1alpha1.CFOrg

	BeforeEach(func() {
		org = &korifiv1alpha1.CFOrg{
			ObjectMeta: metav1.ObjectMeta{
				Name:      uuid.NewString(),
				Namespace: namespace,
			},
			Spec: korifiv1alpha1.CFOrgSpec{
				DisplayName: uuid.NewString(),
			},
		}
	})

	Describe("on create", func() {
		It("allows a resource with no user labels", func() {
			Expect(adminClient.Create(ctx, org)).To(Succeed())
			Eventually(func(g Gomega) {
				g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(org), org)).To(Succeed())
				g.Expect(org.Annotations).To(HaveKey(korifiv1alpha1.LabelSignatureAnnotationKey))
			}).Should(Succeed())
		})

		It("allows a resource with arbitrary user labels", func() {
			org.Labels = map[string]string{"user-label": "value"}
			Expect(adminClient.Create(ctx, org)).To(Succeed())
		})

		It("rejects a non-korifi cloudfoundry.org label key", func() {
			org.Labels = map[string]string{"foo.cloudfoundry.org/bar": "baz"}
			Expect(adminClient.Create(ctx, org)).To(MatchError(ContainSubstring("cannot use the cloudfoundry.org domain")))
		})
	})

	Describe("on update", func() {
		JustBeforeEach(func() {
			Expect(adminClient.Create(ctx, org)).To(Succeed())
			Eventually(func(g Gomega) {
				g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(org), org)).To(Succeed())
				g.Expect(org.Annotations).To(HaveKey(korifiv1alpha1.LabelSignatureAnnotationKey))
			}).Should(Succeed())
		})

		It("allows user labels that don't touch korifi labels", func() {
			Expect(k8s.Patch(ctx, adminClient, org, func() {
				if org.Labels == nil {
					org.Labels = map[string]string{}
				}
				org.Labels["my-custom-label"] = "my-value"
			})).To(Succeed())
		})

		It("rejects tampering with a korifi.cloudfoundry.org label", func() {
			originalReservedValue := org.Labels[korifiv1alpha1.CFOrgDisplayNameKey]

			err := k8s.Patch(ctx, adminClient, org, func() {
				if org.Labels == nil {
					org.Labels = map[string]string{}
				}
				org.Labels[korifiv1alpha1.CFOrgDisplayNameKey] = "tampered-value"
			})
			Expect(err).NotTo(HaveOccurred())

			Eventually(func(g Gomega) {
				g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(org), org)).To(Succeed())
				g.Expect(org.Labels).To(HaveKeyWithValue(korifiv1alpha1.CFOrgDisplayNameKey, originalReservedValue))
			}).Should(Succeed())
		})

		It("rejects a non-korifi cloudfoundry.org label key", func() {
			err := k8s.Patch(ctx, adminClient, org, func() {
				if org.Labels == nil {
					org.Labels = map[string]string{}
				}
				org.Labels["foo.cloudfoundry.org/bar"] = "baz"
			})
			Expect(err).To(MatchError(ContainSubstring("cannot use the cloudfoundry.org domain")))
		})
	})
})
