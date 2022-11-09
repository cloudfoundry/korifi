package integration_test

import (
	"context"
	"fmt"
	"strings"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	. "code.cloudfoundry.org/korifi/controllers/controllers/workloads/testutils"
	"code.cloudfoundry.org/korifi/tools/k8s"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	contourv1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("CFRouteReconciler Integration Tests", func() {
	var (
		ctx           context.Context
		testRouteHost string

		testNamespace  string
		testDomainGUID string
		testRouteGUID  string
		testDomainName string
		testFQDN       string

		ns *corev1.Namespace

		cfDomain *korifiv1alpha1.CFDomain
		cfRoute  *korifiv1alpha1.CFRoute
	)

	BeforeEach(func() {
		ctx = context.Background()

		testRouteHost = "test-route-host"

		testNamespace = GenerateGUID()

		ns = &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: testNamespace,
			},
		}
		Expect(k8sClient.Create(ctx, ns)).To(Succeed())

		testDomainGUID = GenerateGUID()
		testDomainName = "a" + GenerateGUID() + ".com"
		testRouteGUID = GenerateGUID()
		testFQDN = fmt.Sprintf("%s.%s", testRouteHost, testDomainName)

		cfDomain = &korifiv1alpha1.CFDomain{
			ObjectMeta: metav1.ObjectMeta{
				Name:      testDomainGUID,
				Namespace: testNamespace,
			},
			Spec: korifiv1alpha1.CFDomainSpec{
				Name: testDomainName,
			},
		}
		Expect(k8sClient.Create(ctx, cfDomain)).To(Succeed())
		Eventually(func() error {
			return k8sClient.Get(
				ctx,
				types.NamespacedName{
					Namespace: testNamespace,
					Name:      testDomainGUID,
				},
				&korifiv1alpha1.CFDomain{},
			)
		}).Should(Succeed())

		cfRoute = &korifiv1alpha1.CFRoute{
			ObjectMeta: metav1.ObjectMeta{
				Name:      testRouteGUID,
				Namespace: testNamespace,
			},
			Spec: korifiv1alpha1.CFRouteSpec{
				Host:     testRouteHost,
				Path:     "/test/path",
				Protocol: "http",
				DomainRef: corev1.ObjectReference{
					Name:      testDomainGUID,
					Namespace: testNamespace,
				},
			},
		}
	})

	AfterEach(func() {
		Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, cfRoute))).To(Succeed())
		Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, cfDomain))).To(Succeed())
		Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, ns))).To(Succeed())
	})

	JustBeforeEach(func() {
		Expect(k8sClient.Create(ctx, cfRoute)).To(Succeed())
	})

	It("reconciles the CFRoute to a root Contour HTTPProxy which includes a proxy for a route destination", func() {
		Eventually(func(g Gomega) {
			var proxy contourv1.HTTPProxy
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: testFQDN, Namespace: testNamespace}, &proxy)).To(Succeed())

			g.Expect(proxy.Spec.VirtualHost.Fqdn).To(Equal(testFQDN))
			g.Expect(proxy.Spec.VirtualHost.TLS.SecretName).To(Equal("korifi-controllers-system/korifi-workloads-ingress-cert"))
			g.Expect(proxy.Spec.Includes).To(ConsistOf(contourv1.Include{
				Name:      testRouteGUID,
				Namespace: testNamespace,
			}))

			g.Expect(proxy.ObjectMeta.OwnerReferences).To(ConsistOf(metav1.OwnerReference{
				APIVersion: "korifi.cloudfoundry.org/v1alpha1",
				Kind:       "CFRoute",
				Name:       cfRoute.Name,
				UID:        cfRoute.GetUID(),
			}))
		}).Should(Succeed())
	})

	It("reconciles the CFRoute to a child proxy with no routes", func() {
		Eventually(func(g Gomega) {
			var proxy contourv1.HTTPProxy
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: testRouteGUID, Namespace: testNamespace}, &proxy)).To(Succeed())
			g.Expect(proxy.Spec.VirtualHost).To(BeNil())
			g.Expect(proxy.Spec.Routes).To(BeEmpty())
			g.Expect(proxy.ObjectMeta.OwnerReferences).To(ConsistOf(metav1.OwnerReference{
				APIVersion: "korifi.cloudfoundry.org/v1alpha1",
				Kind:       "CFRoute",
				Name:       cfRoute.Name,
				UID:        cfRoute.GetUID(),
			}))
		}).Should(Succeed())
	})

	When("the route Host contains upper case characters", func() {
		BeforeEach(func() {
			testRouteHost = "My-App"
			testFQDN = strings.ToLower(fmt.Sprintf("%s.%s", testRouteHost, testDomainName))
		})

		It("reconciles the CFRoute to a root Contour HTTPProxy which includes a proxy for a route destination", func() {
			Eventually(func(g Gomega) {
				var proxy contourv1.HTTPProxy
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: testFQDN, Namespace: testNamespace}, &proxy)).To(Succeed())
				g.Expect(proxy.Spec.VirtualHost.Fqdn).To(Equal(testFQDN))
			})
		})
	})

	When("the CFRoute includes destinations", func() {
		var destinations []korifiv1alpha1.Destination

		BeforeEach(func() {
			destinations = []korifiv1alpha1.Destination{
				{
					GUID: GenerateGUID(),
					AppRef: corev1.LocalObjectReference{
						Name: "the-app-guid",
					},
					ProcessType: "web",
					Port:        80,
					Protocol:    "http1",
				},
			}
			cfRoute.Spec.Destinations = destinations
		})

		It("reconciles the CFRoute to a root Contour HTTPProxy which includes a proxy for a route destination", func() {
			Eventually(func(g Gomega) {
				var proxy contourv1.HTTPProxy
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: testFQDN, Namespace: testNamespace}, &proxy)).To(Succeed())
				g.Expect(proxy.Spec.Includes).To(ConsistOf(contourv1.Include{
					Name:      testRouteGUID,
					Namespace: testNamespace,
				}))
			}).Should(Succeed())
		})

		It("reconciles the CFRoute to a child proxy with a route", func() {
			Eventually(func(g Gomega) {
				var proxy contourv1.HTTPProxy
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: testRouteGUID, Namespace: testNamespace}, &proxy)).To(Succeed())
				g.Expect(proxy.Spec.Routes).To(ConsistOf(contourv1.Route{
					Conditions: []contourv1.MatchCondition{
						{
							Prefix: "/test/path",
						},
					},
					Services: []contourv1.Service{
						{
							Name: fmt.Sprintf("s-%s", cfRoute.Spec.Destinations[0].GUID),
							Port: cfRoute.Spec.Destinations[0].Port,
						},
					},
					EnableWebsockets: true,
				}))
			}).Should(Succeed())
		})

		It("reconciles each destination to a Service for the app", func() {
			serviceName := fmt.Sprintf("s-%s", cfRoute.Spec.Destinations[0].GUID)
			Eventually(func(g Gomega) {
				var svc corev1.Service

				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: serviceName, Namespace: testNamespace}, &svc)).To(Succeed())
				g.Expect(svc.Labels).To(SatisfyAll(
					HaveKeyWithValue("korifi.cloudfoundry.org/app-guid", cfRoute.Spec.Destinations[0].AppRef.Name),
					HaveKeyWithValue("korifi.cloudfoundry.org/route-guid", cfRoute.Name),
				))
				g.Expect(svc.Spec.Selector).To(SatisfyAll(
					HaveLen(2),
					HaveKeyWithValue("korifi.cloudfoundry.org/app-guid", "the-app-guid"),
					HaveKeyWithValue("korifi.cloudfoundry.org/process-type", "web"),
				))
				g.Expect(svc.ObjectMeta.OwnerReferences).To(ConsistOf(metav1.OwnerReference{
					APIVersion: "korifi.cloudfoundry.org/v1alpha1",
					Kind:       "CFRoute",
					Name:       cfRoute.Name,
					UID:        cfRoute.GetUID(),
				}))
			}).Should(Succeed())
		})

		It("adds the FQDN and URI status fields to the CFRoute", func() {
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: testRouteGUID, Namespace: testNamespace}, cfRoute)).To(Succeed())
				g.Expect(cfRoute.Status.FQDN).To(Equal(testFQDN))
				g.Expect(cfRoute.Status.URI).To(Equal(testFQDN + "/test/path"))
			}).Should(Succeed())
		})

		It("adds the Destinations status field to the CFRoute", func() {
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: testRouteGUID, Namespace: testNamespace}, cfRoute)).To(Succeed())
				g.Expect(cfRoute.Status.Destinations).To(Equal(destinations))
			}).Should(Succeed())
		})
	})

	When("the FQDN of a CFRoute is not unique within a space", func() {
		var (
			duplicateRouteGUID string
			duplicateRoute     *korifiv1alpha1.CFRoute
		)

		BeforeEach(func() {
			duplicateRouteGUID = GenerateGUID()
			duplicateRoute = &korifiv1alpha1.CFRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      duplicateRouteGUID,
					Namespace: testNamespace,
				},
				Spec: korifiv1alpha1.CFRouteSpec{
					Host:     testRouteHost,
					Path:     "/",
					Protocol: "http",
					DomainRef: corev1.ObjectReference{
						Name:      testDomainGUID,
						Namespace: testNamespace,
					},
					Destinations: []korifiv1alpha1.Destination{
						{
							GUID: GenerateGUID(),
							AppRef: corev1.LocalObjectReference{
								Name: "app-guid-2",
							},
							ProcessType: "web",
							Port:        80,
							Protocol:    "http1",
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, duplicateRoute)).To(Succeed())

			Eventually(func(g Gomega) {
				var proxy contourv1.HTTPProxy
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: testFQDN, Namespace: testNamespace}, &proxy)).To(Succeed())
			}).Should(Succeed())
		})

		It("reconciles the CFRoute to the existing Contour HTTPProxy with the matching FQDN", func() {
			Eventually(func(g Gomega) {
				var proxy contourv1.HTTPProxy
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: testFQDN, Namespace: testNamespace}, &proxy)).To(Succeed())
				g.Expect(proxy.Spec.Includes).To(ConsistOf([]contourv1.Include{
					{
						Name:      testRouteGUID,
						Namespace: testNamespace,
					},
					{
						Name:      duplicateRouteGUID,
						Namespace: testNamespace,
					},
				}), "HTTPProxy includes mismatch")
			}).Should(Succeed())
		})

		It("reconciles the duplicate CFRoute to a child proxy with a route", func() {
			Eventually(func(g Gomega) {
				var proxy contourv1.HTTPProxy
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: duplicateRouteGUID, Namespace: testNamespace}, &proxy)).To(Succeed())
				g.Expect(proxy.Spec.VirtualHost).To(BeNil())
				g.Expect(proxy.Spec.Routes).To(ConsistOf(contourv1.Route{
					Conditions: []contourv1.MatchCondition{
						{
							Prefix: "/",
						},
					},
					Services: []contourv1.Service{
						{
							Name: fmt.Sprintf("s-%s", duplicateRoute.Spec.Destinations[0].GUID),
							Port: duplicateRoute.Spec.Destinations[0].Port,
						},
					},
					EnableWebsockets: true,
				}))
			}).Should(Succeed())
		})

		When("the duplicate is deleted", func() {
			JustBeforeEach(func() {
				Eventually(func(g Gomega) {
					var proxy contourv1.HTTPProxy
					g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: testFQDN, Namespace: testNamespace}, &proxy)).To(Succeed())
					g.Expect(proxy.Spec.Includes).To(HaveLen(2))
				})
				Expect(k8sClient.Delete(ctx, duplicateRoute)).To(Succeed())
			})

			It("removes it from the proxy includes list", func() {
				Eventually(func(g Gomega) {
					var proxy contourv1.HTTPProxy
					g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: testFQDN, Namespace: testNamespace}, &proxy)).To(Succeed())
					g.Expect(proxy.Spec.Includes).To(ConsistOf(contourv1.Include{
						Name:      testRouteGUID,
						Namespace: testNamespace,
					}))
				}).Should(Succeed())
			})
		})
	})

	When("a destination is added to a CFRoute", func() {
		BeforeEach(func() {
			cfRoute.Spec.Destinations = []korifiv1alpha1.Destination{
				{
					GUID: GenerateGUID(),
					AppRef: corev1.LocalObjectReference{
						Name: "the-app-guid",
					},
					ProcessType: "web",
					Port:        80,
					Protocol:    "http1",
				},
			}
		})

		JustBeforeEach(func() {
			Eventually(func(g Gomega) {
				var proxy contourv1.HTTPProxy
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: testFQDN, Namespace: testNamespace}, &proxy)).To(Succeed())
			}).Should(Succeed())

			Expect(k8s.Patch(ctx, k8sClient, cfRoute, func() {
				cfRoute.Spec.Destinations = append(cfRoute.Spec.Destinations, korifiv1alpha1.Destination{
					GUID: GenerateGUID(),
					AppRef: corev1.LocalObjectReference{
						Name: "app-guid-2",
					},
					ProcessType: "web",
					Port:        8080,
					Protocol:    "http1",
				})
			})).To(Succeed())
		})

		It("reconciles the CFRoute to a child proxy with a route", func() {
			Eventually(func(g Gomega) {
				var proxy contourv1.HTTPProxy
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: testRouteGUID, Namespace: testNamespace}, &proxy)).To(Succeed())
				g.Expect(proxy.Spec.Routes).To(ConsistOf([]contourv1.Route{
					{
						Conditions: []contourv1.MatchCondition{{Prefix: "/test/path"}},
						Services: []contourv1.Service{
							{
								Name: fmt.Sprintf("s-%s", cfRoute.Spec.Destinations[0].GUID),
								Port: cfRoute.Spec.Destinations[0].Port,
							},
							{
								Name: fmt.Sprintf("s-%s", cfRoute.Spec.Destinations[1].GUID),
								Port: cfRoute.Spec.Destinations[1].Port,
							},
						},
						EnableWebsockets: true,
					},
				}))
			}).Should(Succeed())
		})

		It("reconciles the new destination to a Service for the app", func() {
			serviceName := fmt.Sprintf("s-%s", cfRoute.Spec.Destinations[1].GUID)

			Eventually(func(g Gomega) {
				var svc corev1.Service
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: serviceName, Namespace: testNamespace}, &svc)).To(Succeed())
				g.Expect(svc.Labels).To(SatisfyAll(
					HaveKeyWithValue("korifi.cloudfoundry.org/app-guid", cfRoute.Spec.Destinations[1].AppRef.Name),
					HaveKeyWithValue("korifi.cloudfoundry.org/route-guid", cfRoute.Name),
				))
				g.Expect(svc.Spec.Selector).To(SatisfyAll(
					HaveLen(2),
					HaveKeyWithValue("korifi.cloudfoundry.org/app-guid", cfRoute.Spec.Destinations[1].AppRef.Name),
					HaveKeyWithValue("korifi.cloudfoundry.org/process-type", cfRoute.Spec.Destinations[1].ProcessType),
				))
				g.Expect(svc.ObjectMeta.OwnerReferences).To(ConsistOf([]metav1.OwnerReference{
					{
						APIVersion: "korifi.cloudfoundry.org/v1alpha1",
						Kind:       "CFRoute",
						Name:       cfRoute.Name,
						UID:        cfRoute.GetUID(),
					},
				}))
			})
		})
	})

	When("a destination is removed from a CFRoute", func() {
		var serviceName string

		BeforeEach(func() {
			cfRoute.Spec.Destinations = []korifiv1alpha1.Destination{
				{
					GUID: GenerateGUID(),
					AppRef: corev1.LocalObjectReference{
						Name: "the-app-guid",
					},
					ProcessType: "web",
					Port:        80,
					Protocol:    "http1",
				},
			}
		})

		JustBeforeEach(func() {
			Eventually(func(g Gomega) {
				var proxy contourv1.HTTPProxy
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: testFQDN, Namespace: testNamespace}, &proxy)).To(Succeed())

				var routeProxy contourv1.HTTPProxy
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: testRouteGUID, Namespace: testNamespace}, &routeProxy)).To(Succeed())
				g.Expect(routeProxy.Spec.Routes).To(HaveLen(1))
				g.Expect(routeProxy.Spec.Routes[0].Services).To(HaveLen(1))

				serviceName = routeProxy.Spec.Routes[0].Services[0].Name
				var svc corev1.Service
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: serviceName, Namespace: testNamespace}, &svc)).To(Succeed())
			}).Should(Succeed())

			Expect(k8s.Patch(ctx, k8sClient, cfRoute, func() {
				cfRoute.Spec.Destinations = []korifiv1alpha1.Destination{}
			})).To(Succeed())
		})

		It("deletes the Route on the HTTP proxy", func() {
			Eventually(func(g Gomega) {
				var routeProxy contourv1.HTTPProxy
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: testRouteGUID, Namespace: testNamespace}, &routeProxy)).To(Succeed())
				g.Expect(routeProxy.Spec.Routes).To(BeEmpty())
			}).Should(Succeed())
		})

		It("deletes the corresponding sevice", func() {
			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: serviceName, Namespace: testNamespace}, new(corev1.Service))
				g.Expect(errors.IsNotFound(err)).To(BeTrue())
			}).Should(Succeed())
		})

		It("does not delete the FQDN proxy", func() {
			Consistently(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: testFQDN, Namespace: testNamespace}, new(contourv1.HTTPProxy))).To(Succeed())
			}).Should(Succeed())
		})
	})
})
