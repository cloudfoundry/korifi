package domains_test

import (
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("CFDomainReconciler Integration Tests", func() {
	var cfDomain *korifiv1alpha1.CFDomain

	BeforeEach(func() {
		domainNamespace := uuid.NewString()
		Expect(adminClient.Create(ctx, &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: domainNamespace,
			},
		})).To(Succeed())

		cfDomain = &korifiv1alpha1.CFDomain{
			ObjectMeta: metav1.ObjectMeta{
				Name:      uuid.NewString(),
				Namespace: domainNamespace,
				Finalizers: []string{
					korifiv1alpha1.CFDomainFinalizerName,
				},
			},
			Spec: korifiv1alpha1.CFDomainSpec{
				Name: "a" + uuid.NewString() + ".com",
			},
		}
		Expect(adminClient.Create(ctx, cfDomain)).To(Succeed())
	})

	It("sets the domain Ready status", func() {
		Eventually(func(g Gomega) {
			g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(cfDomain), cfDomain)).To(Succeed())
			g.Expect(meta.IsStatusConditionTrue(cfDomain.Status.Conditions, korifiv1alpha1.StatusConditionReady)).To(BeTrue())
		}).Should(Succeed())
	})

	Describe("finalization", func() {
		var (
			route1Namespace string
			route2Namespace string
		)
		BeforeEach(func() {
			route1Namespace = uuid.NewString()
			Expect(adminClient.Create(ctx, &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: route1Namespace,
				},
			})).To(Succeed())

			route2Namespace = uuid.NewString()
			Expect(adminClient.Create(ctx, &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: route2Namespace,
				},
			})).To(Succeed())

			Expect(adminClient.Create(ctx, &korifiv1alpha1.CFRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      uuid.NewString(),
					Namespace: route1Namespace,
				},
				Spec: korifiv1alpha1.CFRouteSpec{
					DomainRef: corev1.ObjectReference{
						Name:      cfDomain.Name,
						Namespace: cfDomain.Namespace,
					},
				},
			})).To(Succeed())

			Expect(adminClient.Create(ctx, &korifiv1alpha1.CFRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      uuid.NewString(),
					Namespace: route2Namespace,
				},
				Spec: korifiv1alpha1.CFRouteSpec{
					DomainRef: corev1.ObjectReference{
						Name:      cfDomain.Name,
						Namespace: cfDomain.Namespace,
					},
				},
			})).To(Succeed())
		})

		JustBeforeEach(func() {
			Expect(adminClient.Delete(ctx, cfDomain)).To(Succeed())
		})

		It("deletes the domain routes", func() {
			Eventually(func(g Gomega) {
				routes := &korifiv1alpha1.CFRouteList{}

				g.Expect(adminClient.List(ctx, routes, client.InNamespace(route1Namespace))).To(Succeed())
				g.Expect(routes.Items).To(BeEmpty())

				g.Expect(adminClient.List(ctx, routes, client.InNamespace(route2Namespace))).To(Succeed())
				g.Expect(routes.Items).To(BeEmpty())
			}).Should(Succeed())
		})
	})
})
