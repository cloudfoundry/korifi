package repositories_test

import (
	"context"
	"testing"

	"code.cloudfoundry.org/cf-k8s-api/repositories"
	networkingv1alpha1 "code.cloudfoundry.org/cf-k8s-controllers/apis/networking/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	. "github.com/onsi/gomega"
	"github.com/sclevine/spec"
)

var _ = SuiteDescribe("Route API Shim", func(t *testing.T, when spec.G, it spec.S) {
	g := NewWithT(t)

	when("multiple CFRoute resources exist", func() {
		var (
			cfRoute1 *networkingv1alpha1.CFRoute
			cfRoute2 *networkingv1alpha1.CFRoute
		)

		it.Before(func() {
			ctx := context.Background()

			cfRoute1 = &networkingv1alpha1.CFRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "route-id-1",
					Namespace: "default",
				},
				Spec: networkingv1alpha1.CFRouteSpec{
					Host:     "my-subdomain-1",
					Path:     "",
					Protocol: "http",
					DomainRef: corev1.LocalObjectReference{
						Name: "my-domain",
					},
					Destinations: []networkingv1alpha1.Destination{},
				},
			}
			g.Expect(k8sClient.Create(ctx, cfRoute1)).To(Succeed())

			cfRoute2 = &networkingv1alpha1.CFRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "route-id-2",
					Namespace: "default",
				},
				Spec: networkingv1alpha1.CFRouteSpec{
					Host:     "my-subdomain-2",
					Path:     "",
					Protocol: "http",
					DomainRef: corev1.LocalObjectReference{
						Name: "my-domain",
					},
					Destinations: []networkingv1alpha1.Destination{},
				},
			}
			g.Expect(k8sClient.Create(ctx, cfRoute2)).To(Succeed())
		})

		it("fetches the CFRoute CR we're looking for", func() {
			routeRepo := repositories.RouteRepo{}
			routeClient, err := routeRepo.ConfigureClient(k8sConfig)
			g.Expect(err).ToNot(HaveOccurred())

			route := repositories.RouteRecord{}
			route, err = routeRepo.FetchRoute(routeClient, "route-id-1")
			g.Expect(err).ToNot(HaveOccurred())

			g.Expect(route.GUID).To(Equal("route-id-1"))
			g.Expect(route.Host).To(Equal("my-subdomain-1"))
			g.Expect(route.SpaceGUID).To(Equal("default"))
			g.Expect(route.Path).To(Equal(""))
			g.Expect(route.Protocol).To(Equal("http"))
			g.Expect(route.Destinations).To(Equal([]repositories.Destination{}))
			g.Expect(route.DomainRef.GUID).To(Equal("my-domain"))
		})

		it.After(func() {
			ctx := context.Background()
			g.Expect(k8sClient.Delete(ctx, cfRoute1)).To(Succeed())
			g.Expect(k8sClient.Delete(ctx, cfRoute2)).To(Succeed())
		})
	})

	when("no CFRoute exists", func() {
		it("returns an error", func() {
			routeRepo := repositories.RouteRepo{}
			routeClient, err := routeRepo.ConfigureClient(k8sConfig)
			g.Expect(err).ToNot(HaveOccurred())

			_, err = routeRepo.FetchRoute(routeClient, "non-existent-route-guid")
			g.Expect(err).To(MatchError("not found"))
		})
	})

	when("multiple CFRoute resources exist across namespaces with the same name", func() {
		var (
			cfRoute1            *networkingv1alpha1.CFRoute
			cfRoute2            *networkingv1alpha1.CFRoute
			nonDefaultNamespace *corev1.Namespace
		)

		it.Before(func() {
			ctx := context.Background()
			// Create second namespace aside from default within which to create a duplicate route
			nonDefaultNamespace = &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "non-default-namespace"}}
			g.Expect(k8sClient.Create(ctx, nonDefaultNamespace)).To(Succeed())

			cfRoute1 = &networkingv1alpha1.CFRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "non-unique-route-id",
					Namespace: "default",
				},
				Spec: networkingv1alpha1.CFRouteSpec{
					Host:     "",
					Path:     "",
					Protocol: "http",
					DomainRef: corev1.LocalObjectReference{
						Name: "my-domain",
					},
					Destinations: []networkingv1alpha1.Destination{},
				},
			}
			g.Expect(k8sClient.Create(ctx, cfRoute1)).To(Succeed())

			cfRoute2 = &networkingv1alpha1.CFRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "non-unique-route-id",
					Namespace: "non-default-namespace",
				},
				Spec: networkingv1alpha1.CFRouteSpec{
					Host:     "",
					Path:     "",
					Protocol: "http",
					DomainRef: corev1.LocalObjectReference{
						Name: "my-domain",
					},
					Destinations: []networkingv1alpha1.Destination{},
				},
			}
			g.Expect(k8sClient.Create(ctx, cfRoute2)).To(Succeed())
		})

		it("returns an error", func() {
			routeRepo := repositories.RouteRepo{}
			routeClient, err := routeRepo.ConfigureClient(k8sConfig)
			g.Expect(err).ToNot(HaveOccurred())

			// Looks like we can continue doing state-based setup for the time being
			// Assumption: when unit testing, we can ignore webhooks that might turn the uniqueness constraint into a race condition
			// If assumption is invalidated, we can implement the setup by mocking a fake client to return the non-unique ids

			_, err = routeRepo.FetchRoute(routeClient, "non-unique-route-id")
			g.Expect(err).To(MatchError("duplicate route GUID exists"))
		})

		it.After(func() {
			ctx := context.Background()
			g.Expect(k8sClient.Delete(ctx, cfRoute1)).To(Succeed())
			g.Expect(k8sClient.Delete(ctx, cfRoute2)).To(Succeed())
			g.Expect(k8sClient.Delete(ctx, nonDefaultNamespace)).To(Succeed())
		})
	})
})
