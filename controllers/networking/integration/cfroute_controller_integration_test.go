package integration_test

import (
	"context"
	"fmt"
	"time"

	networkingv1alpha1 "code.cloudfoundry.org/cf-k8s-controllers/apis/networking/v1alpha1"
	. "code.cloudfoundry.org/cf-k8s-controllers/controllers/workloads/testutils"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	contourv1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("CFRouteReconciler Integration Tests", func() {
	const (
		testRouteHost = "test-route-host"
	)

	var (
		testNamespace  string
		testDomainGUID string
		testRouteGUID  string
		testDomainName string
		testFQDN       string

		ns *corev1.Namespace

		cfDomain *networkingv1alpha1.CFDomain
		cfRoute  *networkingv1alpha1.CFRoute
	)

	BeforeEach(func() {
		ctx := context.Background()

		testNamespace = GenerateGUID()

		ns = &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: testNamespace,
			},
		}
		Expect(k8sClient.Create(ctx, ns)).To(Succeed())

		testDomainGUID = GenerateGUID()
		testDomainName = GenerateGUID()
		testFQDN = fmt.Sprintf("%s.%s", testRouteHost, testDomainName)

		cfDomain = &networkingv1alpha1.CFDomain{
			ObjectMeta: metav1.ObjectMeta{
				Name: testDomainGUID,
			},
			Spec: networkingv1alpha1.CFDomainSpec{
				Name: testDomainName,
			},
		}
		Expect(k8sClient.Create(ctx, cfDomain)).To(Succeed())
	})

	AfterEach(func() {
		ctx := context.Background()

		Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, cfRoute))).To(Succeed())
		Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, cfDomain))).To(Succeed())
		Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, ns))).To(Succeed())
	})

	When("the CFRoute does not include any destinations", func() {
		BeforeEach(func() {
			ctx := context.Background()

			testRouteGUID = GenerateGUID()

			cfRoute = &networkingv1alpha1.CFRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testRouteGUID,
					Namespace: testNamespace,
				},
				Spec: networkingv1alpha1.CFRouteSpec{
					Host:     testRouteHost,
					Path:     "",
					Protocol: "http",
					DomainRef: corev1.LocalObjectReference{
						Name: testDomainGUID,
					},
					Destinations: []networkingv1alpha1.Destination{},
				},
			}
			Expect(k8sClient.Create(ctx, cfRoute)).To(Succeed())
		})

		It("reconciles the CFRoute to a root Contour HTTPProxy which includes a proxy for a route destination", func() {
			ctx := context.Background()

			Eventually(func() string {
				var proxy contourv1.HTTPProxy
				err := k8sClient.Get(ctx, types.NamespacedName{Name: testFQDN, Namespace: testNamespace}, &proxy)
				Expect(client.IgnoreNotFound(err)).NotTo(HaveOccurred(), fmt.Sprintf("Failed to get HTTPProxy/%s in namespace %s", testFQDN, testNamespace))
				return proxy.Name
			}, 2*time.Second).ShouldNot(BeEmpty(), fmt.Sprintf("Timed out waiting for HTTPProxy/%s in namespace %s to be created", testFQDN, testNamespace))

			var proxy contourv1.HTTPProxy
			err := k8sClient.Get(ctx, types.NamespacedName{Name: testFQDN, Namespace: testNamespace}, &proxy)
			Expect(err).NotTo(HaveOccurred())

			Expect(proxy.Spec.VirtualHost.Fqdn).To(Equal(testFQDN), "HTTPProxy FQDN mismatch")
			Expect(proxy.Spec.Includes).To(HaveLen(1), "HTTPProxy doesn't have the expected number of includes")
			Expect(proxy.Spec.Includes[0]).To(Equal(contourv1.Include{
				Name:      testRouteGUID,
				Namespace: testNamespace,
			}), "HTTPProxy include does not match proxy for route destinations")

			Expect(proxy.ObjectMeta.OwnerReferences).To(ConsistOf([]metav1.OwnerReference{
				{
					APIVersion: "networking.cloudfoundry.org/v1alpha1",
					Kind:       "CFRoute",
					Name:       cfRoute.Name,
					UID:        cfRoute.GetUID(),
				},
			}))
		})

		It("reconciles the CFRoute to a child proxy with no routes", func() {
			ctx := context.Background()

			Eventually(func() string {
				var proxy contourv1.HTTPProxy
				err := k8sClient.Get(ctx, types.NamespacedName{Name: testRouteGUID, Namespace: testNamespace}, &proxy)
				Expect(client.IgnoreNotFound(err)).NotTo(HaveOccurred(), fmt.Sprintf("Failed to get HTTPProxy/%s in namespace %s", testRouteGUID, testNamespace))
				return proxy.GetName()
			}, 2*time.Second).ShouldNot(BeEmpty(), fmt.Sprintf("Timed out waiting for HTTPProxy/%s in namespace %s to be created", testRouteGUID, testNamespace))

			var proxy contourv1.HTTPProxy
			err := k8sClient.Get(ctx, types.NamespacedName{Name: testRouteGUID, Namespace: testNamespace}, &proxy)
			Expect(err).NotTo(HaveOccurred())

			Expect(proxy.Spec.VirtualHost).To(BeNil(), "VirtualHost set on non-root HTTPProxy")
			Expect(proxy.Spec.Routes).To(HaveLen(0), "HTTPProxy doesn't have the expected number of routes")

			Expect(proxy.ObjectMeta.OwnerReferences).To(ConsistOf([]metav1.OwnerReference{
				{
					APIVersion: "networking.cloudfoundry.org/v1alpha1",
					Kind:       "CFRoute",
					Name:       cfRoute.Name,
					UID:        cfRoute.GetUID(),
				},
			}))
		})

		It("adds a finalizer to the CFRoute", func() {
			ctx := context.Background()

			Eventually(func() []string {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: testRouteGUID, Namespace: testNamespace}, cfRoute)
				Expect(err).NotTo(HaveOccurred())
				return cfRoute.ObjectMeta.Finalizers
			}, 2*time.Second).Should(ConsistOf([]string{
				"cfRoute.networking.cloudfoundry.org",
			}))
		})
	})

	When("the CFRoute includes destinations", func() {
		BeforeEach(func() {
			ctx := context.Background()

			testRouteGUID = GenerateGUID()

			cfRoute = &networkingv1alpha1.CFRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testRouteGUID,
					Namespace: testNamespace,
				},
				Spec: networkingv1alpha1.CFRouteSpec{
					Host:     testRouteHost,
					Path:     "/",
					Protocol: "http",
					DomainRef: corev1.LocalObjectReference{
						Name: testDomainGUID,
					},
					Destinations: []networkingv1alpha1.Destination{
						{
							AppRef: corev1.LocalObjectReference{
								Name: "the-app-guid",
							},
							ProcessType: "web",
							Port:        80,
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, cfRoute)).To(Succeed())
		})

		It("reconciles the CFRoute to a root Contour HTTPProxy which includes a proxy for a route destination", func() {
			ctx := context.Background()

			Eventually(func() string {
				var proxy contourv1.HTTPProxy
				err := k8sClient.Get(ctx, types.NamespacedName{Name: testFQDN, Namespace: testNamespace}, &proxy)
				Expect(client.IgnoreNotFound(err)).NotTo(HaveOccurred(), fmt.Sprintf("Failed to get HTTPProxy/%s in namespace %s", testFQDN, testNamespace))
				return proxy.GetName()
			}, 2*time.Second).ShouldNot(BeEmpty(), fmt.Sprintf("Timed out waiting for HTTPProxy/%s in namespace %s to be created", testFQDN, testNamespace))

			var proxy contourv1.HTTPProxy
			err := k8sClient.Get(ctx, types.NamespacedName{Name: testFQDN, Namespace: testNamespace}, &proxy)
			Expect(err).NotTo(HaveOccurred())

			Expect(proxy.Spec.Includes).To(HaveLen(1), "HTTPProxy doesn't have the expected number of includes")
			Expect(proxy.Spec.Includes[0]).To(Equal(contourv1.Include{
				Name:      testRouteGUID,
				Namespace: testNamespace,
			}), "HTTPProxy include does not match proxy for route destinations")
		})

		It("reconciles the CFRoute to a child proxy with a route", func() {
			ctx := context.Background()

			Eventually(func() string {
				var proxy contourv1.HTTPProxy
				err := k8sClient.Get(ctx, types.NamespacedName{Name: testRouteGUID, Namespace: testNamespace}, &proxy)
				Expect(client.IgnoreNotFound(err)).NotTo(HaveOccurred(), fmt.Sprintf("Failed to get HTTPProxy/%s in namespace %s", testRouteGUID, testNamespace))
				return proxy.GetName()
			}, 2*time.Second).ShouldNot(BeEmpty(), fmt.Sprintf("Timed out waiting for HTTPProxy/%s in namespace %s to be created", testRouteGUID, testNamespace))

			var proxy contourv1.HTTPProxy
			err := k8sClient.Get(ctx, types.NamespacedName{Name: testRouteGUID, Namespace: testNamespace}, &proxy)
			Expect(err).NotTo(HaveOccurred())

			Expect(proxy.Spec.Routes).To(HaveLen(1), "HTTPProxy doesn't have the expected number of routes")
			Expect(proxy.Spec.Routes[0]).To(Equal(contourv1.Route{
				Conditions: []contourv1.MatchCondition{
					{
						Prefix: "/",
					},
				},
				Services: []contourv1.Service{
					{
						Name: fmt.Sprintf("s-%s-%s", cfRoute.Spec.Destinations[0].AppRef.Name, cfRoute.Spec.Destinations[0].ProcessType),
						Port: cfRoute.Spec.Destinations[0].Port,
					},
				},
			}), "HTTPProxy route does not match destination")
		})

		It("reconciles each destination to a Service", func() {
			ctx := context.Background()

			serviceName := fmt.Sprintf("s-%s-%s", cfRoute.Spec.Destinations[0].AppRef.Name, cfRoute.Spec.Destinations[0].ProcessType)
			Eventually(func() string {
				var svc corev1.Service
				err := k8sClient.Get(ctx, types.NamespacedName{Name: serviceName, Namespace: testNamespace}, &svc)
				Expect(client.IgnoreNotFound(err)).NotTo(HaveOccurred(), fmt.Sprintf("Failed to get Service/%s in namespace %s", serviceName, testNamespace))
				return svc.GetName()
			}, 2*time.Second).ShouldNot(BeEmpty(), fmt.Sprintf("Timed out waiting for Service/%s in namespace %s to be created", serviceName, testNamespace))

			var svc corev1.Service
			err := k8sClient.Get(ctx, types.NamespacedName{Name: serviceName, Namespace: testNamespace}, &svc)
			Expect(err).NotTo(HaveOccurred())

			Expect(svc.Spec.Selector).To(Equal(map[string]string{
				"workloads.cloudfoundry.org/app-guid":     "the-app-guid",
				"workloads.cloudfoundry.org/process-type": "web",
			}))

			Expect(svc.ObjectMeta.OwnerReferences).To(ConsistOf([]metav1.OwnerReference{
				{
					APIVersion: "networking.cloudfoundry.org/v1alpha1",
					Kind:       "CFRoute",
					Name:       cfRoute.Name,
					UID:        cfRoute.GetUID(),
				},
			}))
		})
	})

	When("the FQDN of a CFRoute is not unique within a space", func() {
		var (
			duplicateRouteGUID string
			duplicateRoute     *networkingv1alpha1.CFRoute
		)

		BeforeEach(func() {
			ctx := context.Background()

			testRouteGUID = GenerateGUID()

			cfRoute = &networkingv1alpha1.CFRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testRouteGUID,
					Namespace: testNamespace,
				},
				Spec: networkingv1alpha1.CFRouteSpec{
					Host:     testRouteHost,
					Path:     "/",
					Protocol: "http",
					DomainRef: corev1.LocalObjectReference{
						Name: testDomainGUID,
					},
					Destinations: []networkingv1alpha1.Destination{
						{
							AppRef: corev1.LocalObjectReference{
								Name: "app-guid-1",
							},
							ProcessType: "web",
							Port:        80,
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, cfRoute)).To(Succeed())
			Eventually(func() string {
				var proxy contourv1.HTTPProxy
				err := k8sClient.Get(ctx, types.NamespacedName{Name: testFQDN, Namespace: testNamespace}, &proxy)
				Expect(client.IgnoreNotFound(err)).NotTo(HaveOccurred(), fmt.Sprintf("Failed to get HTTPProxy/%s in namespace %s", testFQDN, testNamespace))
				return proxy.GetName()
			}, 2*time.Second).ShouldNot(BeEmpty(), fmt.Sprintf("Timed out waiting for HTTPProxy/%s in namespace %s to be created", testFQDN, testNamespace))

			duplicateRouteGUID = GenerateGUID()

			duplicateRoute = &networkingv1alpha1.CFRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      duplicateRouteGUID,
					Namespace: testNamespace,
				},
				Spec: networkingv1alpha1.CFRouteSpec{
					Host:     testRouteHost,
					Path:     "/",
					Protocol: "http",
					DomainRef: corev1.LocalObjectReference{
						Name: testDomainGUID,
					},
					Destinations: []networkingv1alpha1.Destination{
						{
							AppRef: corev1.LocalObjectReference{
								Name: "app-guid-2",
							},
							ProcessType: "web",
							Port:        80,
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, duplicateRoute)).To(Succeed())
		})

		It("reconciles the CFRoute to the existing Contour HTTPProxy with the matching FQDN", func() {
			ctx := context.Background()

			Eventually(func() int {
				var proxy contourv1.HTTPProxy
				err := k8sClient.Get(ctx, types.NamespacedName{Name: testFQDN, Namespace: testNamespace}, &proxy)
				Expect(client.IgnoreNotFound(err)).NotTo(HaveOccurred(), fmt.Sprintf("Failed to get HTTPProxy/%s in namespace %s", testFQDN, testNamespace))
				return len(proxy.Spec.Includes)
			}, 2*time.Second).Should(Equal(2), fmt.Sprintf("Timed out waiting for HTTPProxy/%s in namespace %s to include HTTPProxy/%s", testFQDN, testNamespace, duplicateRouteGUID))

			var proxy contourv1.HTTPProxy
			err := k8sClient.Get(ctx, types.NamespacedName{Name: testFQDN, Namespace: testNamespace}, &proxy)
			Expect(err).NotTo(HaveOccurred())

			Expect(proxy.Spec.Includes).To(ConsistOf([]contourv1.Include{
				{
					Name:      testRouteGUID,
					Namespace: testNamespace,
				},
				{
					Name:      duplicateRouteGUID,
					Namespace: testNamespace,
				},
			}), "HTTPProxy includes mismatch")
		})

		It("reconciles the duplicate CFRoute to a child proxy with a route", func() {
			ctx := context.Background()

			Eventually(func() string {
				var proxy contourv1.HTTPProxy
				err := k8sClient.Get(ctx, types.NamespacedName{Name: duplicateRouteGUID, Namespace: testNamespace}, &proxy)
				Expect(client.IgnoreNotFound(err)).NotTo(HaveOccurred(), fmt.Sprintf("Failed to get HTTPProxy/%s in namespace %s", duplicateRouteGUID, testNamespace))
				return proxy.GetName()
			}, 2*time.Second).ShouldNot(BeEmpty(), fmt.Sprintf("Timed out waiting for HTTPProxy/%s in namespace %s to be created", duplicateRouteGUID, testNamespace))

			var proxy contourv1.HTTPProxy
			err := k8sClient.Get(ctx, types.NamespacedName{Name: duplicateRouteGUID, Namespace: testNamespace}, &proxy)
			Expect(err).NotTo(HaveOccurred())

			Expect(proxy.Spec.VirtualHost).To(BeNil(), "VirtualHost set on non-root HTTPProxy")
			Expect(proxy.Spec.Routes).To(HaveLen(1), "HTTPProxy doesn't have the expected number of routes")
			Expect(proxy.Spec.Routes[0]).To(Equal(contourv1.Route{
				Conditions: []contourv1.MatchCondition{
					{
						Prefix: "/",
					},
				},
				Services: []contourv1.Service{
					{
						Name: fmt.Sprintf("s-%s-%s", duplicateRoute.Spec.Destinations[0].AppRef.Name, duplicateRoute.Spec.Destinations[0].ProcessType),
						Port: duplicateRoute.Spec.Destinations[0].Port,
					},
				},
			}), "HTTPProxy route does not match destination")
		})
	})

	When("a destination is added to a CFRoute", func() {
		BeforeEach(func() {
			ctx := context.Background()

			testRouteGUID = GenerateGUID()

			cfRoute = &networkingv1alpha1.CFRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testRouteGUID,
					Namespace: testNamespace,
				},
				Spec: networkingv1alpha1.CFRouteSpec{
					Host:     testRouteHost,
					Path:     "/",
					Protocol: "http",
					DomainRef: corev1.LocalObjectReference{
						Name: testDomainGUID,
					},
					Destinations: []networkingv1alpha1.Destination{
						{
							AppRef: corev1.LocalObjectReference{
								Name: "the-app-guid",
							},
							ProcessType: "web",
							Port:        80,
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, cfRoute)).To(Succeed())
			Eventually(func() string {
				var proxy contourv1.HTTPProxy
				err := k8sClient.Get(ctx, types.NamespacedName{Name: testFQDN, Namespace: testNamespace}, &proxy)
				Expect(client.IgnoreNotFound(err)).NotTo(HaveOccurred(), fmt.Sprintf("Failed to get HTTPProxy/%s in namespace %s", testFQDN, testNamespace))
				return proxy.GetName()
			}, 2*time.Second).ShouldNot(BeEmpty(), fmt.Sprintf("Timed out waiting for HTTPProxy/%s in namespace %s to be created", testFQDN, testNamespace))

			cfRoute.Spec.Destinations = append(cfRoute.Spec.Destinations, networkingv1alpha1.Destination{
				AppRef: corev1.LocalObjectReference{
					Name: "app-guid-2",
				},
				ProcessType: "web",
				Port:        8080,
			})
			Expect(k8sClient.Update(ctx, cfRoute)).To(Succeed())
		})

		It("reconciles the CFRoute to a child proxy with a route", func() {
			ctx := context.Background()

			Eventually(func() int {
				var proxy contourv1.HTTPProxy
				err := k8sClient.Get(ctx, types.NamespacedName{Name: testRouteGUID, Namespace: testNamespace}, &proxy)
				Expect(client.IgnoreNotFound(err)).NotTo(HaveOccurred(), fmt.Sprintf("Failed to get HTTPProxy/%s in namespace %s", testRouteGUID, testNamespace))
				return len(proxy.Spec.Routes)
			}, 2*time.Second).Should(Equal(2), fmt.Sprintf("Timed out waiting for HTTPProxy/%s in namespace %s to be updated", testRouteGUID, testNamespace))

			var proxy contourv1.HTTPProxy
			err := k8sClient.Get(ctx, types.NamespacedName{Name: testRouteGUID, Namespace: testNamespace}, &proxy)
			Expect(err).NotTo(HaveOccurred())

			Expect(proxy.Spec.Routes).To(ConsistOf([]contourv1.Route{
				{
					Conditions: []contourv1.MatchCondition{
						{
							Prefix: "/",
						},
					},
					Services: []contourv1.Service{
						{
							Name: fmt.Sprintf("s-%s-%s", cfRoute.Spec.Destinations[0].AppRef.Name, cfRoute.Spec.Destinations[0].ProcessType),
							Port: cfRoute.Spec.Destinations[0].Port,
						},
					},
				},
				{
					Conditions: []contourv1.MatchCondition{
						{
							Prefix: "/",
						},
					},
					Services: []contourv1.Service{
						{
							Name: fmt.Sprintf("s-%s-%s", cfRoute.Spec.Destinations[1].AppRef.Name, cfRoute.Spec.Destinations[1].ProcessType),
							Port: cfRoute.Spec.Destinations[1].Port,
						},
					},
				},
			}), "HTTPProxy routes mismatch")
		})
	})

	When("a destination is removed from a CFRoute", func() {
		// TODO: separate story to handle this properly
	})
})
