package networking_test

import (
	"context"
	"fmt"
	"strings"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	. "code.cloudfoundry.org/korifi/controllers/controllers/workloads/testutils"
	"code.cloudfoundry.org/korifi/tools"
	"code.cloudfoundry.org/korifi/tools/k8s"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
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
		cfApp    *korifiv1alpha1.CFApp
	)

	BeforeEach(func() {
		ctx = context.Background()

		testNamespace = GenerateGUID()

		ns = &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: testNamespace,
			},
		}
		Expect(adminClient.Create(ctx, ns)).To(Succeed())

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
		Expect(adminClient.Create(ctx, cfDomain)).To(Succeed())

		cfApp = &korifiv1alpha1.CFApp{
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
		Expect(adminClient.Create(ctx, cfApp)).To(Succeed())

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
		Expect(client.IgnoreNotFound(adminClient.Delete(ctx, ns))).To(Succeed())
	})

	JustBeforeEach(func() {
		Expect(adminClient.Create(ctx, cfRoute)).To(Succeed())
	})

	It("creates an fqdn HTTPProxy owned by the cfroute", func() {
		Eventually(func(g Gomega) {
			var proxy contourv1.HTTPProxy
			g.Expect(adminClient.Get(ctx, types.NamespacedName{Name: fqdnProxyName(), Namespace: testNamespace}, &proxy)).To(Succeed())

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

	It("creates an http proxy with no routes owned by the cfroute", func() {
		Eventually(func(g Gomega) {
			var proxy contourv1.HTTPProxy
			g.Expect(adminClient.Get(ctx, types.NamespacedName{Name: testRouteGUID, Namespace: testNamespace}, &proxy)).To(Succeed())
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

	It("sets a valid status on the cfroute", func() {
		Eventually(func(g Gomega) {
			g.Expect(adminClient.Get(ctx, types.NamespacedName{Name: testRouteGUID, Namespace: testNamespace}, cfRoute)).To(Succeed())
			g.Expect(cfRoute.Status.CurrentStatus).To(Equal(korifiv1alpha1.ValidStatus))
			g.Expect(cfRoute.Status.FQDN).To(Equal(fqdnProxyName()))
			g.Expect(cfRoute.Status.URI).To(Equal(fqdnProxyName() + "/test/path"))
			g.Expect(cfRoute.Status.Destinations).To(BeEmpty())
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
					Port:        tools.PtrTo(80),
				},
			}
		})

		It("creates an http proxy", func() {
			Eventually(func(g Gomega) {
				var proxy contourv1.HTTPProxy
				g.Expect(adminClient.Get(ctx, types.NamespacedName{Name: testRouteGUID, Namespace: testNamespace}, &proxy)).To(Succeed())
				g.Expect(proxy.Spec.Routes).To(ConsistOf(contourv1.Route{
					Conditions: []contourv1.MatchCondition{{
						Prefix: "/test/path",
					}},
					Services: []contourv1.Service{{
						Name: fmt.Sprintf("s-%s", cfRoute.Spec.Destinations[0].GUID),
						Port: *cfRoute.Spec.Destinations[0].Port,
					}},
					EnableWebsockets: true,
				}))
			}).Should(Succeed())
		})

		It("creates a service for the destination", func() {
			serviceName := fmt.Sprintf("s-%s", cfRoute.Spec.Destinations[0].GUID)
			Eventually(func(g Gomega) {
				var svc corev1.Service

				g.Expect(adminClient.Get(ctx, types.NamespacedName{Name: serviceName, Namespace: testNamespace}, &svc)).To(Succeed())
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

		It("sets effective destinations to the cfroute status", func() {
			Eventually(func(g Gomega) {
				g.Expect(adminClient.Get(ctx, types.NamespacedName{Name: testRouteGUID, Namespace: testNamespace}, cfRoute)).To(Succeed())
				g.Expect(cfRoute.Status.Destinations).To(ConsistOf(korifiv1alpha1.Destination{
					GUID:        cfRoute.Spec.Destinations[0].GUID,
					Port:        tools.PtrTo(80),
					Protocol:    tools.PtrTo("http1"),
					AppRef:      cfRoute.Spec.Destinations[0].AppRef,
					ProcessType: "web",
				}))
			}).Should(Succeed())
		})

		When("the destination has no port set", func() {
			BeforeEach(func() {
				cfRoute.Spec.Destinations[0].Port = nil
			})

			It("does not create a service", func() {
				serviceName := fmt.Sprintf("s-%s", cfRoute.Spec.Destinations[0].GUID)
				Consistently(func(g Gomega) {
					err := adminClient.Get(ctx, types.NamespacedName{Name: serviceName, Namespace: testNamespace}, &corev1.Service{})
					g.Expect(err).To(MatchError(ContainSubstring("not found")))
				}).Should(Succeed())
			})

			It("creates an http proxy with no routes", func() {
				Eventually(func(g Gomega) {
					var proxy contourv1.HTTPProxy
					g.Expect(adminClient.Get(ctx, types.NamespacedName{Name: testRouteGUID, Namespace: testNamespace}, &proxy)).To(Succeed())
					g.Expect(proxy.Spec.Routes).To(BeEmpty())
				}).Should(Succeed())
			})

			It("does not add destinations to the status", func() {
				Consistently(func(g Gomega) {
					g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(cfRoute), cfRoute)).To(Succeed())
					g.Expect(cfRoute.Status.Destinations).To(BeEmpty())
				}).Should(Succeed())
			})

			When("a build for the app appears", func() {
				var (
					dropletPorts []int32
					cfBuild      *korifiv1alpha1.CFBuild
				)

				BeforeEach(func() {
					dropletPorts = []int32{1234}
				})

				JustBeforeEach(func() {
					cfBuild = &korifiv1alpha1.CFBuild{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: testNamespace,
							Name:      uuid.NewString(),
						},
						Spec: korifiv1alpha1.CFBuildSpec{
							Lifecycle: korifiv1alpha1.Lifecycle{
								Type: "docker",
								Data: korifiv1alpha1.LifecycleData{},
							},
							AppRef: corev1.LocalObjectReference{
								Name: testAppGUID,
							},
						},
					}
					Expect(adminClient.Create(ctx, cfBuild)).To(Succeed())
					Expect(k8s.Patch(ctx, adminClient, cfBuild, func() {
						cfBuild.Status = korifiv1alpha1.CFBuildStatus{
							Droplet: &korifiv1alpha1.BuildDropletStatus{
								Ports: dropletPorts,
							},
						}
					})).To(Succeed())
				})

				It("does not add the destination to the route status", func() {
					Consistently(func(g Gomega) {
						g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(cfRoute), cfRoute)).To(Succeed())
						g.Expect(cfRoute.Status.Destinations).To(BeEmpty())
					}).Should(Succeed())
				})

				When("the build is set as app current droplet", func() {
					JustBeforeEach(func() {
						Expect(k8s.Patch(ctx, adminClient, cfApp, func() {
							cfApp.Spec.CurrentDropletRef = corev1.LocalObjectReference{
								Name: cfBuild.Name,
							}
						})).To(Succeed())
					})

					It("uses the ports from the droplet", func() {
						Eventually(func(g Gomega) {
							g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(cfRoute), cfRoute)).To(Succeed())
							g.Expect(cfRoute.Status.Destinations).To(ConsistOf(MatchFields(IgnoreExtras, Fields{
								"Port":        PointTo(BeNumerically("==", 1234)),
								"Protocol":    PointTo(Equal("http1")),
								"ProcessType": Equal("web"),
							})))
						}).Should(Succeed())
					})

					When("the droplet has no ports", func() {
						BeforeEach(func() {
							dropletPorts = []int32{}
						})

						It("defaults to 8080", func() {
							Eventually(func(g Gomega) {
								g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(cfRoute), cfRoute)).To(Succeed())
								g.Expect(cfRoute.Status.Destinations).To(ConsistOf(MatchFields(IgnoreExtras, Fields{
									"Port":     PointTo(BeNumerically("==", 8080)),
									"Protocol": PointTo(Equal("http1")),
								})))
							}).Should(Succeed())
						})
					})
				})
			})
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
							Port:        tools.PtrTo(80),
						},
					},
				},
			}
			Expect(adminClient.Create(ctx, anotherRoute)).To(Succeed())

			Eventually(func(g Gomega) {
				var proxy contourv1.HTTPProxy
				g.Expect(adminClient.Get(ctx, types.NamespacedName{Name: fqdnProxyName(), Namespace: testNamespace}, &proxy)).To(Succeed())
			}).Should(Succeed())
		})

		It("adds another include to the contour FQDN proxy", func() {
			Eventually(func(g Gomega) {
				var proxy contourv1.HTTPProxy
				g.Expect(adminClient.Get(ctx, types.NamespacedName{Name: fqdnProxyName(), Namespace: testNamespace}, &proxy)).To(Succeed())
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

		When("one of the routes is deleted", func() {
			JustBeforeEach(func() {
				Eventually(func(g Gomega) {
					var proxy contourv1.HTTPProxy
					g.Expect(adminClient.Get(ctx, types.NamespacedName{Name: fqdnProxyName(), Namespace: testNamespace}, &proxy)).To(Succeed())
					g.Expect(proxy.Spec.Includes).To(HaveLen(2))
				}).Should(Succeed())
				Expect(adminClient.Delete(ctx, anotherRoute)).To(Succeed())
			})

			It("removes it from the FQDN proxy includes list", func() {
				Eventually(func(g Gomega) {
					var proxy contourv1.HTTPProxy
					g.Expect(adminClient.Get(ctx, types.NamespacedName{Name: fqdnProxyName(), Namespace: testNamespace}, &proxy)).To(Succeed())
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
						Name: testAppGUID,
					},
					ProcessType: "web",
					Port:        tools.PtrTo(80),
				},
			}
		})

		JustBeforeEach(func() {
			Eventually(func(g Gomega) {
				var proxy contourv1.HTTPProxy
				g.Expect(adminClient.Get(ctx, types.NamespacedName{Name: fqdnProxyName(), Namespace: testNamespace}, &proxy)).To(Succeed())
			}).Should(Succeed())

			Expect(k8s.Patch(ctx, adminClient, cfRoute, func() {
				cfRoute.Spec.Destinations = append(cfRoute.Spec.Destinations, korifiv1alpha1.Destination{
					GUID: GenerateGUID(),
					AppRef: corev1.LocalObjectReference{
						Name: testAppGUID,
					},
					ProcessType: "web",
					Port:        tools.PtrTo(8080),
				})
			})).To(Succeed())
		})

		It("adds a service to the http proxy", func() {
			Eventually(func(g Gomega) {
				var proxy contourv1.HTTPProxy
				g.Expect(adminClient.Get(ctx, types.NamespacedName{Name: testRouteGUID, Namespace: testNamespace}, &proxy)).To(Succeed())
				g.Expect(proxy.Spec.Routes).To(ConsistOf([]contourv1.Route{{
					Conditions: []contourv1.MatchCondition{{Prefix: "/test/path"}},
					Services: []contourv1.Service{{
						Name: fmt.Sprintf("s-%s", cfRoute.Spec.Destinations[0].GUID),
						Port: *cfRoute.Spec.Destinations[0].Port,
					}, {
						Name: fmt.Sprintf("s-%s", cfRoute.Spec.Destinations[1].GUID),
						Port: *cfRoute.Spec.Destinations[1].Port,
					}},
					EnableWebsockets: true,
				}}))
			}).Should(Succeed())
		})

		It("creates a service for the destination", func() {
			Eventually(func(g Gomega) {
				g.Expect(adminClient.Get(
					ctx,
					types.NamespacedName{
						Name:      fmt.Sprintf("s-%s", cfRoute.Spec.Destinations[1].GUID),
						Namespace: testNamespace,
					},
					&corev1.Service{},
				)).To(Succeed())
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
					Port:        tools.PtrTo(80),
				},
			}
		})

		JustBeforeEach(func() {
			Eventually(func(g Gomega) {
				var proxy contourv1.HTTPProxy
				g.Expect(adminClient.Get(ctx, types.NamespacedName{Name: fqdnProxyName(), Namespace: testNamespace}, &proxy)).To(Succeed())

				var routeProxy contourv1.HTTPProxy
				g.Expect(adminClient.Get(ctx, types.NamespacedName{Name: testRouteGUID, Namespace: testNamespace}, &routeProxy)).To(Succeed())
				g.Expect(routeProxy.Spec.Routes).To(HaveLen(1))
				g.Expect(routeProxy.Spec.Routes[0].Services).To(HaveLen(1))

				serviceName = routeProxy.Spec.Routes[0].Services[0].Name
				var svc corev1.Service
				g.Expect(adminClient.Get(ctx, types.NamespacedName{Name: serviceName, Namespace: testNamespace}, &svc)).To(Succeed())
			}).Should(Succeed())

			Expect(k8s.Patch(ctx, adminClient, cfRoute, func() {
				cfRoute.Spec.Destinations = []korifiv1alpha1.Destination{}
			})).To(Succeed())
		})

		It("deletes the Route from the HTTP proxy", func() {
			Eventually(func(g Gomega) {
				var routeProxy contourv1.HTTPProxy
				g.Expect(adminClient.Get(ctx, types.NamespacedName{Name: testRouteGUID, Namespace: testNamespace}, &routeProxy)).To(Succeed())
				g.Expect(routeProxy.Spec.Routes).To(BeEmpty())
			}).Should(Succeed())
		})

		It("deletes the corresponding service", func() {
			Eventually(func(g Gomega) {
				err := adminClient.Get(ctx, types.NamespacedName{Name: serviceName, Namespace: testNamespace}, new(corev1.Service))
				g.Expect(errors.IsNotFound(err)).To(BeTrue())
			}).Should(Succeed())
		})

		It("does not delete the FQDN proxy", func() {
			Consistently(func(g Gomega) {
				g.Expect(adminClient.Get(ctx, types.NamespacedName{Name: fqdnProxyName(), Namespace: testNamespace}, new(contourv1.HTTPProxy))).To(Succeed())
			}).Should(Succeed())
		})
	})
})
