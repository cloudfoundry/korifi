package v1alpha1_test

import (
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("CFDomainMutatingWebhook", func() {
	var cfDomain *korifiv1alpha1.CFDomain

	BeforeEach(func() {
		cfDomain = &korifiv1alpha1.CFDomain{
			ObjectMeta: metav1.ObjectMeta{
				Name:      uuid.NewString(),
				Namespace: namespace,
				Labels: map[string]string{
					"foo": "bar",
				},
			},
			Spec: korifiv1alpha1.CFDomainSpec{
				Name: uuid.NewString(),
			},
		}
	})

	JustBeforeEach(func() {
		Expect(adminClient.Create(ctx, cfDomain)).To(Succeed())
	})

	It("sets the domain name label", func() {
		Expect(cfDomain.Labels).To(HaveKeyWithValue(korifiv1alpha1.CFDomainNameLabelKey, cfDomain.Spec.Name))
	})

	It("preserves all other labels", func() {
		Expect(cfDomain.Labels).To(HaveKeyWithValue("foo", "bar"))
	})
})
