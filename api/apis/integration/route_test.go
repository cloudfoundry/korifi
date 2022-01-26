package integration_test

import (
	"context"
	"net/http"
	"strings"

	. "code.cloudfoundry.org/cf-k8s-controllers/api/apis"
	"code.cloudfoundry.org/cf-k8s-controllers/api/apis/fake"
	"code.cloudfoundry.org/cf-k8s-controllers/api/authorization"
	"code.cloudfoundry.org/cf-k8s-controllers/api/repositories"
	networkingv1alpha1 "code.cloudfoundry.org/cf-k8s-controllers/controllers/apis/networking/v1alpha1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

var _ = Describe("Route Handler", func() {
	var (
		apiHandler         *RouteHandler
		namespace          *corev1.Namespace
		spaceDeveloperRole *rbacv1.ClusterRole
	)

	BeforeEach(func() {
		clientFactory := repositories.NewUnprivilegedClientFactory(k8sConfig)
		identityProvider := new(fake.IdentityProvider)
		nsPermissions := authorization.NewNamespacePermissions(k8sClient, identityProvider, "root-ns")
		appRepo := repositories.NewAppRepo(k8sClient, clientFactory, nsPermissions)
		routeRepo := repositories.NewRouteRepo(k8sClient, clientFactory)
		domainRepo := repositories.NewDomainRepo(k8sClient)

		apiHandler = NewRouteHandler(
			logf.Log.WithName("TestRouteHandler"),
			*serverURL,
			routeRepo,
			domainRepo,
			appRepo,
		)
		apiHandler.RegisterRoutes(router)

		namespaceGUID := generateGUID()
		namespace = &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespaceGUID}}
		Expect(
			k8sClient.Create(ctx, namespace),
		).To(Succeed())

		spaceDeveloperRole = createClusterRole(ctx, repositories.SpaceDeveloperClusterRoleRules)
	})

	AfterEach(func() {
		Expect(k8sClient.Delete(ctx, namespace)).To(Succeed())
	})

	Describe("DELETE /v3/route endpoint", func() {
		When("a space developer calls delete on a route in namespace they have permission for", func() {
			var (
				routeGUID  string
				domainGUID string
				cfRoute    *networkingv1alpha1.CFRoute
			)

			BeforeEach(func() {
				createRoleBinding(ctx, userName, spaceDeveloperRole.Name, namespace.Name)

				routeGUID = generateGUID()
				domainGUID = generateGUID()

				cfDomain := &networkingv1alpha1.CFDomain{
					ObjectMeta: metav1.ObjectMeta{
						Name: domainGUID,
					},
					Spec: networkingv1alpha1.CFDomainSpec{
						Name: "foo.domain",
					},
				}
				Expect(
					k8sClient.Create(ctx, cfDomain),
				).To(Succeed())

				cfRoute = &networkingv1alpha1.CFRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      routeGUID,
						Namespace: namespace.Name,
					},
					Spec: networkingv1alpha1.CFRouteSpec{
						Host:     "my-subdomain-1",
						Path:     "",
						Protocol: "http",
						DomainRef: corev1.LocalObjectReference{
							Name: domainGUID,
						},
						Destinations: []networkingv1alpha1.Destination{
							{
								GUID: "destination-guid",
								Port: 8080,
								AppRef: corev1.LocalObjectReference{
									Name: "some-app-guid",
								},
								ProcessType: "web",
								Protocol:    "http1",
							},
						},
					},
				}
				Expect(
					k8sClient.Create(ctx, cfRoute),
				).To(Succeed())

				Eventually(func() error {
					route := &networkingv1alpha1.CFRoute{}
					return k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace.Name, Name: routeGUID}, route)
				}).ShouldNot(HaveOccurred())

				var err error
				req, err = http.NewRequestWithContext(ctx, "DELETE", serverURI("/v3/routes/"+routeGUID), strings.NewReader(""))
				Expect(err).NotTo(HaveOccurred())

				req.Header.Add("Content-type", "application/json")
			})

			JustBeforeEach(func() {
				router.ServeHTTP(rr, req)
			})

			It("returns 202 and eventually deletes the route", func() {
				testCtx := context.Background()
				Expect(rr.Code).To(Equal(202))

				Eventually(func() error {
					route := &networkingv1alpha1.CFRoute{}
					return k8sClient.Get(testCtx, client.ObjectKey{Namespace: namespace.Name, Name: routeGUID}, route)
				}).Should(MatchError(ContainSubstring("not found")))
			})
		})
	})
})
