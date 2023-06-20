package networking_test

import (
	"context"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	. "code.cloudfoundry.org/korifi/controllers/controllers/workloads/testutils"
	"sigs.k8s.io/controller-runtime/pkg/client"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("CFDomainReconciler Integration Tests", func() {
	var (
		ctx             context.Context
		testDomainName  string
		testDomainGUID  string
		domainNamespace string
		route1Namespace string
		route2Namespace string

		cfDomain *korifiv1alpha1.CFDomain
	)

	BeforeEach(func() {
		ctx = context.Background()

		domainNamespace = GenerateGUID()
		Expect(k8sClient.Create(ctx, &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: domainNamespace,
			},
		})).To(Succeed())

		route1Namespace = GenerateGUID()
		Expect(k8sClient.Create(ctx, &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: route1Namespace,
			},
		})).To(Succeed())

		route2Namespace = GenerateGUID()
		Expect(k8sClient.Create(ctx, &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: route2Namespace,
			},
		})).To(Succeed())

		testDomainGUID = GenerateGUID()
		testDomainName = "a" + GenerateGUID() + ".com"
		cfDomain = &korifiv1alpha1.CFDomain{
			ObjectMeta: metav1.ObjectMeta{
				Name:      testDomainGUID,
				Namespace: domainNamespace,
			},
			Spec: korifiv1alpha1.CFDomainSpec{
				Name: testDomainName,
			},
		}
		Expect(k8sClient.Create(ctx, cfDomain)).To(Succeed())

		createValidRoute(ctx, &korifiv1alpha1.CFRoute{
			ObjectMeta: metav1.ObjectMeta{
				Name:      GenerateGUID(),
				Namespace: route1Namespace,
			},
			Spec: korifiv1alpha1.CFRouteSpec{
				Host:     "test-route-host-1",
				Path:     "/test/path/1",
				Protocol: "http",
				DomainRef: corev1.ObjectReference{
					Name:      testDomainGUID,
					Namespace: domainNamespace,
				},
			},
		})

		createValidRoute(ctx, &korifiv1alpha1.CFRoute{
			ObjectMeta: metav1.ObjectMeta{
				Name:      GenerateGUID(),
				Namespace: route2Namespace,
			},
			Spec: korifiv1alpha1.CFRouteSpec{
				Host:     "test-route-host-2",
				Path:     "/test/path/2",
				Protocol: "http",
				DomainRef: corev1.ObjectReference{
					Name:      testDomainGUID,
					Namespace: domainNamespace,
				},
			},
		})
	})

	When("a domain is deleted", func() {
		JustBeforeEach(func() {
			Expect(k8sClient.Delete(ctx, cfDomain)).To(Succeed())
		})

		It("deletes the domain routes", func() {
			Eventually(func(g Gomega) {
				routes := &korifiv1alpha1.CFRouteList{}

				g.Expect(k8sClient.List(ctx, routes, client.InNamespace(route1Namespace))).To(Succeed())
				g.Expect(routes.Items).To(BeEmpty())

				g.Expect(k8sClient.List(ctx, routes, client.InNamespace(route2Namespace))).To(Succeed())
				g.Expect(routes.Items).To(BeEmpty())
			}).Should(Succeed())
		})

		It("writes a log message", func() {
			Eventually(logOutput).Should(gbytes.Say("finalizer removed"))
		})
	})
})

func createValidRoute(ctx context.Context, route *korifiv1alpha1.CFRoute) {
	Expect(k8sClient.Create(ctx, route)).To(Succeed())
	Eventually(func(g Gomega) {
		createdRoute := &korifiv1alpha1.CFRoute{}
		g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(route), createdRoute)).To(Succeed())
		g.Expect(meta.IsStatusConditionTrue(createdRoute.Status.Conditions, "Valid")).To(BeTrue())
	}).Should(Succeed())
}
