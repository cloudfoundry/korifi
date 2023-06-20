package networking_test

import (
	"context"
	"fmt"
	"strings"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	. "code.cloudfoundry.org/korifi/controllers/controllers/workloads/testutils"
	"code.cloudfoundry.org/korifi/tools"
	"code.cloudfoundry.org/korifi/tools/k8s"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	contourv1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("CFRouteReconciler Integration Tests", func() {
	var (
		ctx context.Context

		testNamespace  string
		testDomainGUID string
		testRouteGUID  string
		testAppGUID    string

		ns *corev1.Namespace

		cfDomain *korifiv1alpha1.CFDomain
		cfRoute  *korifiv1alpha1.CFRoute
	)

	BeforeEach(func() {
		ctx = context.Background()

		testNamespace = GenerateGUID()

		ns = &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: testNamespace,
			},
		}
		Expect(k8sClient.Create(ctx, ns)).To(Succeed())

		testDomainGUID = GenerateGUID()
		testRouteGUID = GenerateGUID()
		testAppGUID = GenerateGUID()

		cfDomain = &korifiv1alpha1.CFDomain{
			ObjectMeta: metav1.ObjectMeta{
				Name:      testDomainGUID,
				Namespace: testNamespace,
			},
			Spec: korifiv1alpha1.CFDomainSpec{
				Name: "a" + GenerateGUID() + ".com",
			},
		}
		Expect(k8sClient.Create(ctx, cfDomain)).To(Succeed())

		cfApp := &korifiv1alpha1.CFApp{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: testNamespace,
				Name:      testAppGUID,
			},
			Spec: korifiv1alpha1.CFAppSpec{
				Lifecycle: korifiv1alpha1.Lifecycle{
					Type: "buildpack",
				},
				DesiredState: "STARTED",
				DisplayName:  testAppGUID,
			},
		}
		Expect(k8sClient.Create(ctx, cfApp)).To(Succeed())

		cfRoute = &korifiv1alpha1.CFRoute{
			ObjectMeta: metav1.ObjectMeta{
				Name:      testRouteGUID,
				Namespace: testNamespace,
			},
			Spec: korifiv1alpha1.CFRouteSpec{
				Host:     "test-route-host",
				Path:     "/test/path",
				Protocol: "http",
				DomainRef: corev1.ObjectReference{
					Name:      testDomainGUID,
					Namespace: testNamespace,
				},
			},
		}
	})

	fqdnProxyName := func() string {
		return strings.ToLower(fmt.Sprintf("%s.%s", cfRoute.Spec.Host, cfDomain.Spec.Name))
	}

	AfterEach(func() {
		Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, ns))).To(Succeed())
	})

	JustBeforeEach(func() {
		Expect(k8sClient.Create(ctx, cfRoute)).To(Succeed())
	})

	It("reconciles the CFRoute to a root Contour HTTPProxy which includes a proxy for a route destination", func() {
		Eventually(func(g Gomega) {
			var proxy contourv1.HTTPProxy
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: fqdnProxyName(), Namespace: testNamespace}, &proxy)).To(Succeed())

			g.Expect(proxy.Spec.VirtualHost.Fqdn).To(Equal(fqdnProxyName()))
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
				APIVersion:         "korifi.cloudfoundry.org/v1alpha1",
				Kind:               "CFRoute",
				Name:               cfRoute.Name,
				UID:                cfRoute.GetUID(),
				Controller:         tools.PtrTo(true),
				BlockOwnerDeletion: tools.PtrTo(true),
			}))
		}).Should(Succeed())
	})

	It("sets the ObservedGeneration status field", func() {
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: testRouteGUID, Namespace: testNamespace}, cfRoute)).To(Succeed())
			g.Expect(cfRoute.Status.ObservedGeneration).To(Equal(cfRoute.Generation))
		}).Should(Succeed())
	})

	When("the CFRoute includes destinations", func() {
		BeforeEach(func() {
			cfRoute.Spec.Destinations = []korifiv1alpha1.Destination{
				{
					GUID: GenerateGUID(),
					AppRef: corev1.LocalObjectReference{
						Name: testAppGUID,
					},
					ProcessType: "web",
					Port:        80,
					Protocol:    "http1",
				},
			}
		})

		It("reconciles the CFRoute to a root Contour HTTPProxy which includes a proxy for a route destination", func() {
			Eventually(func(g Gomega) {
				var proxy contourv1.HTTPProxy
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: fqdnProxyName(), Namespace: testNamespace}, &proxy)).To(Succeed())
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
					HaveKeyWithValue("korifi.cloudfoundry.org/app-guid", testAppGUID),
					HaveKeyWithValue("korifi.cloudfoundry.org/process-type", "web"),
				))
				g.Expect(svc.ObjectMeta.OwnerReferences).To(ConsistOf(metav1.OwnerReference{
					APIVersion:         "korifi.cloudfoundry.org/v1alpha1",
					Kind:               "CFRoute",
					Name:               cfRoute.Name,
					UID:                cfRoute.GetUID(),
					Controller:         tools.PtrTo(true),
					BlockOwnerDeletion: tools.PtrTo(true),
				}))
			}).Should(Succeed())
		})

		It("adds the FQDN and URI status fields to the CFRoute", func() {
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: testRouteGUID, Namespace: testNamespace}, cfRoute)).To(Succeed())
				g.Expect(cfRoute.Status.FQDN).To(Equal(fqdnProxyName()))
				g.Expect(cfRoute.Status.URI).To(Equal(fqdnProxyName() + "/test/path"))
			}).Should(Succeed())
		})

		It("adds the Destinations status field to the CFRoute", func() {
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: testRouteGUID, Namespace: testNamespace}, cfRoute)).To(Succeed())
				g.Expect(cfRoute.Status.Destinations).To(Equal(cfRoute.Spec.Destinations))
			}).Should(Succeed())
		})
	})

	When("there are multiple routes in the space", func() {
		var (
			anotherRouteGUID string
			anotherRoute     *korifiv1alpha1.CFRoute
		)

		BeforeEach(func() {
			anotherRouteGUID = GenerateGUID()
			anotherRoute = &korifiv1alpha1.CFRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      anotherRouteGUID,
					Namespace: testNamespace,
				},
				Spec: korifiv1alpha1.CFRouteSpec{
					Host:     cfRoute.Spec.Host,
					Path:     "/test/another",
					Protocol: "http",
					DomainRef: corev1.ObjectReference{
						Name:      testDomainGUID,
						Namespace: testNamespace,
					},
					Destinations: []korifiv1alpha1.Destination{
						{
							GUID: GenerateGUID(),
							AppRef: corev1.LocalObjectReference{
								Name: testAppGUID,
							},
							ProcessType: "web",
							Port:        80,
							Protocol:    "http1",
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, anotherRoute)).To(Succeed())

			Eventually(func(g Gomega) {
				var proxy contourv1.HTTPProxy
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: fqdnProxyName(), Namespace: testNamespace}, &proxy)).To(Succeed())
			}).Should(Succeed())
		})

		It("adds another include to the contour FQDN proxy", func() {
			Eventually(func(g Gomega) {
				var proxy contourv1.HTTPProxy
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: fqdnProxyName(), Namespace: testNamespace}, &proxy)).To(Succeed())
				g.Expect(proxy.Spec.Includes).To(ConsistOf([]contourv1.Include{
					{
						Name:      testRouteGUID,
						Namespace: testNamespace,
					},
					{
						Name:      anotherRouteGUID,
						Namespace: testNamespace,
					},
				}), "HTTPProxy includes mismatch")
			}).Should(Succeed())
		})

		It("reconciles them to a child contour proxy with a route", func() {
			Eventually(func(g Gomega) {
				var proxy contourv1.HTTPProxy
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: anotherRouteGUID, Namespace: testNamespace}, &proxy)).To(Succeed())
				g.Expect(proxy.Spec.VirtualHost).To(BeNil())
				g.Expect(proxy.Spec.Routes).To(ConsistOf(contourv1.Route{
					Conditions: []contourv1.MatchCondition{
						{
							Prefix: "/test/another",
						},
					},
					Services: []contourv1.Service{
						{
							Name: fmt.Sprintf("s-%s", anotherRoute.Spec.Destinations[0].GUID),
							Port: anotherRoute.Spec.Destinations[0].Port,
						},
					},
					EnableWebsockets: true,
				}))
			}).Should(Succeed())
		})

		When("one of the routes is deleted", func() {
			JustBeforeEach(func() {
				Eventually(func(g Gomega) {
					var proxy contourv1.HTTPProxy
					g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: fqdnProxyName(), Namespace: testNamespace}, &proxy)).To(Succeed())
					g.Expect(proxy.Spec.Includes).To(HaveLen(2))
				}).Should(Succeed())
				Expect(k8sClient.Delete(ctx, anotherRoute)).To(Succeed())
			})

			It("removes it from the FQDN proxy includes list", func() {
				Eventually(func(g Gomega) {
					var proxy contourv1.HTTPProxy
					g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: fqdnProxyName(), Namespace: testNamespace}, &proxy)).To(Succeed())
					g.Expect(proxy.Spec.Includes).To(ConsistOf(contourv1.Include{
						Name:      testRouteGUID,
						Namespace: testNamespace,
					}))
				}).Should(Succeed())
			})

			It("writes a log message", func() {
				Eventually(logOutput).Should(gbytes.Say("finalizer removed"))
			})
		})
	})

	When("a destination is added to a CFRoute", func() {
		BeforeEach(func() {
			cfRoute.Spec.Destinations = []korifiv1alpha1.Destination{
				{
					GUID: GenerateGUID(),
					AppRef: corev1.LocalObjectReference{
						Name: testAppGUID,
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
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: fqdnProxyName(), Namespace: testNamespace}, &proxy)).To(Succeed())
			}).Should(Succeed())

			Expect(k8s.Patch(ctx, k8sClient, cfRoute, func() {
				cfRoute.Spec.Destinations = append(cfRoute.Spec.Destinations, korifiv1alpha1.Destination{
					GUID: GenerateGUID(),
					AppRef: corev1.LocalObjectReference{
						Name: testAppGUID,
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
						APIVersion:         "korifi.cloudfoundry.org/v1alpha1",
						Kind:               "CFRoute",
						Name:               cfRoute.Name,
						UID:                cfRoute.GetUID(),
						Controller:         tools.PtrTo(true),
						BlockOwnerDeletion: tools.PtrTo(true),
					},
				}))
			}).Should(Succeed())
		})
	})

	When("a destination is removed from a CFRoute", func() {
		var serviceName string

		BeforeEach(func() {
			cfRoute.Spec.Destinations = []korifiv1alpha1.Destination{
				{
					GUID: GenerateGUID(),
					AppRef: corev1.LocalObjectReference{
						Name: testAppGUID,
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
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: fqdnProxyName(), Namespace: testNamespace}, &proxy)).To(Succeed())

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

		It("deletes the corresponding service", func() {
			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: serviceName, Namespace: testNamespace}, new(corev1.Service))
				g.Expect(errors.IsNotFound(err)).To(BeTrue())
			}).Should(Succeed())
		})

		It("does not delete the FQDN proxy", func() {
			Consistently(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: fqdnProxyName(), Namespace: testNamespace}, new(contourv1.HTTPProxy))).To(Succeed())
			}).Should(Succeed())
		})
	})
})
