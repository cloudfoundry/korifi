package label_indexer_test

import (
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("LabelIndexerWebhook", func() {
	Describe("CFRoute", func() {
		var route *korifiv1alpha1.CFRoute

		BeforeEach(func() {
			route = &korifiv1alpha1.CFRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      uuid.NewString(),
					Namespace: namespace,
				},
				Spec: korifiv1alpha1.CFRouteSpec{
					DomainRef: corev1.ObjectReference{
						Name: "example.com",
					},
				},
			}
		})

		JustBeforeEach(func() {
			Expect(adminClient.Create(ctx, route)).To(Succeed())
		})

		It("labels the CFRoute with the expected index labels", func() {
			Eventually(func(g Gomega) {
				g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(route), route)).To(Succeed())
				g.Expect(route.Labels).To(MatchKeys(IgnoreExtras, Keys{
					korifiv1alpha1.CFDomainGUIDLabelKey: Equal("example.com"),
					korifiv1alpha1.SpaceGUIDKey:         Equal(namespace),
				}))
			}).Should(Succeed())
		})
	})
})
