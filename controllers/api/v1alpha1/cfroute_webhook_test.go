package v1alpha1_test

import (
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	cfDomainLabelKey = "korifi.cloudfoundry.org/domain-guid"
	cfRouteLabelKey  = "korifi.cloudfoundry.org/route-guid"
)

var _ = Describe("CFRouteMutatingWebhook Integration Tests", func() {
	When("a CFRoute record is created", func() {
		var (
			cfDomain *korifiv1alpha1.CFDomain
			cfRoute  *korifiv1alpha1.CFRoute
		)

		BeforeEach(func() {
			cfDomain = &korifiv1alpha1.CFDomain{
				ObjectMeta: metav1.ObjectMeta{
					Name:      uuid.NewString(),
					Namespace: namespace,
				},
				Spec: korifiv1alpha1.CFDomainSpec{
					Name: "a" + uuid.NewString() + ".com",
				},
			}
			Expect(adminClient.Create(ctx, cfDomain)).To(Succeed())

			cfRoute = &korifiv1alpha1.CFRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      uuid.NewString(),
					Namespace: namespace,
					Labels:    map[string]string{"foo": "bar"},
				},
				Spec: korifiv1alpha1.CFRouteSpec{
					Host: "my-host",
					DomainRef: v1.ObjectReference{
						Name:      cfDomain.Name,
						Namespace: namespace,
					},
				},
			}
		})

		JustBeforeEach(func() {
			Expect(adminClient.Create(ctx, cfRoute)).To(Succeed())
		})

		It("adds default labels labels", func() {
			Expect(cfRoute.Labels).To(HaveKeyWithValue(cfRouteLabelKey, cfRoute.Name))
		})

		It("preserves the other labels", func() {
			Expect(cfRoute.Labels).To(HaveKeyWithValue("foo", "bar"))
		})
	})
})
