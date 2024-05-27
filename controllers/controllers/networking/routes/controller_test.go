package routes_test

import (
	"fmt"
	"strings"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/tools"
	"code.cloudfoundry.org/korifi/tools/k8s"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
)

var _ = Describe("CFRouteReconciler Integration Tests", func() {
	var (
		ns *corev1.Namespace

		cfDomain *korifiv1alpha1.CFDomain
		cfRoute  *korifiv1alpha1.CFRoute
	)

	BeforeEach(func() {
		ns = &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: uuid.NewString(),
			},
		}
		Expect(adminClient.Create(ctx, ns)).To(Succeed())

		cfDomain = &korifiv1alpha1.CFDomain{
			ObjectMeta: metav1.ObjectMeta{
				Name:      uuid.NewString(),
				Namespace: ns.Name,
			},
			Spec: korifiv1alpha1.CFDomainSpec{
				Name: "a" + uuid.NewString() + ".com",
			},
		}
		Expect(adminClient.Create(ctx, cfDomain)).To(Succeed())

		cfRoute = &korifiv1alpha1.CFRoute{
			ObjectMeta: metav1.ObjectMeta{
				Name:      uuid.NewString(),
				Namespace: ns.Name,
			},
			Spec: korifiv1alpha1.CFRouteSpec{
				Host:     "test-route-host",
				Path:     "/hello",
				Protocol: "http",
				DomainRef: corev1.ObjectReference{
					Name:      cfDomain.Name,
					Namespace: ns.Name,
				},
			},
		}
	})

	getCfRouteFQDN := func() string {
		return strings.ToLower(fmt.Sprintf("%s.%s", cfRoute.Spec.Host, cfDomain.Spec.Name))
	}

	getHTTPRoute := func() *gatewayv1beta1.HTTPRoute {
		GinkgoHelper()

		httpRoute := &gatewayv1beta1.HTTPRoute{
			ObjectMeta: metav1.ObjectMeta{
				Name:      cfRoute.Name,
				Namespace: cfRoute.Namespace,
			},
		}
		Eventually(func(g Gomega) {
			g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(httpRoute), httpRoute)).To(Succeed())
		}).Should(Succeed())
		return httpRoute
	}

	JustBeforeEach(func() {
		Expect(adminClient.Create(ctx, cfRoute)).To(Succeed())
	})

	It("does not create a HTTPRoute (as there are no destinations)", func() {
		Consistently(func(g Gomega) {
			httpRoutes := &gatewayv1beta1.HTTPRouteList{}
			g.Expect(adminClient.List(ctx, httpRoutes, client.InNamespace(ns.Name))).To(Succeed())
			g.Expect(httpRoutes.Items).To(BeEmpty())
		}).Should(Succeed())
	})

	It("sets a ready condition on the cfroute", func() {
		Eventually(func(g Gomega) {
			g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(cfRoute), cfRoute)).To(Succeed())
			g.Expect(meta.IsStatusConditionTrue(cfRoute.Status.Conditions, korifiv1alpha1.StatusConditionReady)).To(BeTrue())
			g.Expect(cfRoute.Status.FQDN).To(Equal(getCfRouteFQDN()))
			g.Expect(cfRoute.Status.URI).To(Equal(getCfRouteFQDN() + "/hello"))
			g.Expect(cfRoute.Status.Destinations).To(BeEmpty())
		}).Should(Succeed())
	})

	When("the CFRoute includes destinations", func() {
		var cfApp *korifiv1alpha1.CFApp

		BeforeEach(func() {
			cfApp = &korifiv1alpha1.CFApp{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: ns.Name,
					Name:      uuid.NewString(),
				},
				Spec: korifiv1alpha1.CFAppSpec{
					Lifecycle: korifiv1alpha1.Lifecycle{
						Type: "buildpack",
					},
					DesiredState: "STARTED",
					DisplayName:  uuid.NewString(),
				},
			}
			Expect(adminClient.Create(ctx, cfApp)).To(Succeed())

			cfRoute.Spec.Destinations = []korifiv1alpha1.Destination{
				{
					GUID: uuid.NewString(),
					AppRef: corev1.LocalObjectReference{
						Name: cfApp.Name,
					},
					ProcessType: "web",
					Port:        tools.PtrTo(80),
				},
			}
		})

		It("creates an HTTPRoute", func() {
			httpRoute := getHTTPRoute()

			Expect(httpRoute.Spec.ParentRefs).To(ConsistOf(gatewayv1beta1.ParentReference{
				Group:     tools.PtrTo(gatewayv1beta1.Group("gateway.networking.k8s.io")),
				Kind:      tools.PtrTo(gatewayv1beta1.Kind("Gateway")),
				Namespace: tools.PtrTo(gatewayv1beta1.Namespace("korifi-gateway")),
				Name:      gatewayv1beta1.ObjectName("korifi"),
			}))

			Expect(httpRoute.Spec.Hostnames).To(ConsistOf(gatewayv1beta1.Hostname(getCfRouteFQDN())))

			Expect(httpRoute.Spec.Rules).To(HaveLen(1))
			Expect(httpRoute.Spec.Rules[0].Matches).To(ConsistOf(gatewayv1beta1.HTTPRouteMatch{
				Path: &gatewayv1beta1.HTTPPathMatch{
					Type:  tools.PtrTo(gatewayv1.PathMatchPathPrefix),
					Value: tools.PtrTo("/hello"),
				},
			}))

			Expect(httpRoute.Spec.Rules[0].BackendRefs).To(HaveLen(1))
			Expect(httpRoute.Spec.Rules[0].BackendRefs[0].BackendRef.BackendObjectReference).To(Equal(gatewayv1beta1.BackendObjectReference{
				Group: tools.PtrTo(gatewayv1beta1.Group("")),
				Kind:  tools.PtrTo(gatewayv1beta1.Kind("Service")),
				Name:  gatewayv1beta1.ObjectName(fmt.Sprintf("s-%s", cfRoute.Spec.Destinations[0].GUID)),
				Port:  tools.PtrTo(gatewayv1beta1.PortNumber(80)),
			}))

			Expect(httpRoute.OwnerReferences).To(ConsistOf(metav1.OwnerReference{
				APIVersion:         "korifi.cloudfoundry.org/v1alpha1",
				Kind:               "CFRoute",
				Name:               cfRoute.Name,
				UID:                cfRoute.GetUID(),
				Controller:         tools.PtrTo(true),
				BlockOwnerDeletion: tools.PtrTo(true),
			}))
		})

		When("the route's path contains upper case characters", func() {
			BeforeEach(func() {
				cfRoute.Spec.Path = "/Hello"
			})

			It("uses the lowercased path in the httproute name and path match prefix", func() {
				httpRoute := getHTTPRoute()

				Expect(httpRoute.Spec.Rules).To(HaveLen(1))
				Expect(httpRoute.Spec.Rules[0].Matches).To(ConsistOf(gatewayv1beta1.HTTPRouteMatch{
					Path: &gatewayv1beta1.HTTPPathMatch{
						Type:  tools.PtrTo(gatewayv1.PathMatchPathPrefix),
						Value: tools.PtrTo("/hello"),
					},
				}))
			})
		})

		When("the route's host contains upper case characters", func() {
			BeforeEach(func() {
				cfRoute.Spec.Host = "Test-Route"
			})

			It("uses the lowercased host in the httproute name and path match prefix", func() {
				httpRoute := getHTTPRoute()
				Expect(httpRoute.Spec.Hostnames).To(HaveLen(1))
				Expect(httpRoute.Spec.Hostnames).To(HaveCap(1))
				Expect(httpRoute.Spec.Hostnames[0]).To(Equal(gatewayv1beta1.Hostname("test-route.") + gatewayv1.Hostname(cfDomain.Spec.Name)))
			})
		})

		It("creates a service for the destination", func() {
			serviceName := fmt.Sprintf("s-%s", cfRoute.Spec.Destinations[0].GUID)
			Eventually(func(g Gomega) {
				var svc corev1.Service

				g.Expect(adminClient.Get(ctx, types.NamespacedName{Name: serviceName, Namespace: ns.Name}, &svc)).To(Succeed())
				g.Expect(svc.Labels).To(SatisfyAll(
					HaveKeyWithValue("korifi.cloudfoundry.org/app-guid", cfRoute.Spec.Destinations[0].AppRef.Name),
					HaveKeyWithValue("korifi.cloudfoundry.org/route-guid", cfRoute.Name),
				))
				g.Expect(svc.Spec.Selector).To(SatisfyAll(
					HaveLen(2),
					HaveKeyWithValue("korifi.cloudfoundry.org/app-guid", cfApp.Name),
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
				g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(cfRoute), cfRoute)).To(Succeed())
				g.Expect(cfRoute.Status.Destinations).To(ConsistOf(korifiv1alpha1.Destination{
					GUID:        cfRoute.Spec.Destinations[0].GUID,
					Port:        tools.PtrTo(80),
					Protocol:    tools.PtrTo("http1"),
					AppRef:      cfRoute.Spec.Destinations[0].AppRef,
					ProcessType: "web",
				}))
			}).Should(Succeed())
		})

		When("the route's path is empty", func() {
			BeforeEach(func() {
				cfRoute.Spec.Path = ""
			})

			It("defaults the route match path rule to '/'", func() {
				httpRoute := getHTTPRoute()

				Expect(httpRoute.Spec.Rules).To(HaveLen(1))
				Expect(httpRoute.Spec.Rules[0].Matches).To(ConsistOf(gatewayv1beta1.HTTPRouteMatch{
					Path: &gatewayv1beta1.HTTPPathMatch{
						Type:  tools.PtrTo(gatewayv1.PathMatchPathPrefix),
						Value: tools.PtrTo("/"),
					},
				}))
			})

			It("adds a backend ref per destination", func() {
				httpRoute := getHTTPRoute()

				Expect(httpRoute.Spec.Rules[0].BackendRefs).To(HaveLen(1))
				Expect(httpRoute.Spec.Rules[0].BackendRefs[0].BackendRef.BackendObjectReference).To(Equal(gatewayv1beta1.BackendObjectReference{
					Group: tools.PtrTo(gatewayv1beta1.Group("")),
					Kind:  tools.PtrTo(gatewayv1beta1.Kind("Service")),
					Name:  gatewayv1beta1.ObjectName(fmt.Sprintf("s-%s", cfRoute.Spec.Destinations[0].GUID)),
					Port:  tools.PtrTo(gatewayv1beta1.PortNumber(80)),
				}))
			})
		})

		When("the destination has no port set", func() {
			BeforeEach(func() {
				cfRoute.Spec.Destinations[0].Port = nil
			})

			It("does not create a service", func() {
				serviceName := fmt.Sprintf("s-%s", cfRoute.Spec.Destinations[0].GUID)
				Consistently(func(g Gomega) {
					err := adminClient.Get(ctx, types.NamespacedName{Name: serviceName, Namespace: ns.Name}, &corev1.Service{})
					g.Expect(err).To(MatchError(ContainSubstring("not found")))
				}).Should(Succeed())
			})

			It("does not create a HTTPRoute", func() {
				Consistently(func(g Gomega) {
					httpRoutes := &gatewayv1beta1.HTTPRouteList{}
					g.Expect(adminClient.List(ctx, httpRoutes, client.InNamespace(ns.Name))).To(Succeed())
					g.Expect(httpRoutes.Items).To(BeEmpty())
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
							Namespace: ns.Name,
							Name:      uuid.NewString(),
						},
						Spec: korifiv1alpha1.CFBuildSpec{
							Lifecycle: korifiv1alpha1.Lifecycle{
								Type: "docker",
								Data: korifiv1alpha1.LifecycleData{},
							},
							AppRef: corev1.LocalObjectReference{
								Name: cfApp.Name,
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

		When("the destinations are deleted from the route", func() {
			var (
				httpRoute   *gatewayv1beta1.HTTPRoute
				serviceName string
			)

			JustBeforeEach(func() {
				serviceName = fmt.Sprintf("s-%s", cfRoute.Spec.Destinations[0].GUID)
				httpRoute = getHTTPRoute()
				Expect(k8s.Patch(ctx, adminClient, cfRoute, func() {
					cfRoute.Spec.Destinations = []korifiv1alpha1.Destination{}
				})).To(Succeed())
			})

			It("deletes the HTTPRoute", func() {
				Eventually(func(g Gomega) {
					err := adminClient.Get(ctx, client.ObjectKeyFromObject(httpRoute), httpRoute)
					g.Expect(errors.IsNotFound(err)).To(BeTrue())
				}).Should(Succeed())
			})

			It("deletes the corresponding service", func() {
				Eventually(func(g Gomega) {
					err := adminClient.Get(ctx, types.NamespacedName{Name: serviceName, Namespace: ns.Name}, new(corev1.Service))
					g.Expect(errors.IsNotFound(err)).To(BeTrue())
				}).Should(Succeed())
			})
		})
	})

	When("a route has a legacy finalizer", func() {
		BeforeEach(func() {
			cfRoute.Finalizers = []string{
				korifiv1alpha1.CFRouteFinalizerName,
			}
		})

		JustBeforeEach(func() {
			Expect(adminClient.Delete(ctx, cfRoute)).To(Succeed())
		})

		It("is still possible to delete the route", func() {
			Eventually(func(g Gomega) {
				err := adminClient.Get(ctx, client.ObjectKeyFromObject(cfRoute), cfRoute)
				g.Expect(errors.IsNotFound(err)).To(BeTrue())
			}).Should(Succeed())
		})
	})
})
