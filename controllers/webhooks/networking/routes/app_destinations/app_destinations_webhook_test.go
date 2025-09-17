package routes_test

import (
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/tools/k8s"
	. "github.com/onsi/gomega/gstruct"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("AppDestinationsWebhook", func() {
	Describe("CFRoute", func() {
		var (
			route    *korifiv1alpha1.CFRoute
			appGUID  string
			appGUID1 string
		)

		BeforeEach(func() {
			appGUID = uuid.NewString()
			appGUID1 = uuid.NewString()
			route = &korifiv1alpha1.CFRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      uuid.NewString(),
					Namespace: rootNamespace,
				},
				Spec: korifiv1alpha1.CFRouteSpec{
					Host: "example",
					Path: "/example",
					DomainRef: corev1.ObjectReference{
						Name: "example.com",
					},
					Protocol: "",
					Destinations: []korifiv1alpha1.Destination{
						{
							AppRef: corev1.LocalObjectReference{
								Name: appGUID,
							},
						},
						{
							AppRef: corev1.LocalObjectReference{
								Name: appGUID1,
							},
						},
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
				g.Expect(route.Annotations).To(MatchKeys(IgnoreExtras, Keys{
					korifiv1alpha1.CFRouteAppGuidsAnnotationKey: Equal(appGUID + "\n" + appGUID1),
				}))
			}).Should(Succeed())
		})

		When("the route has no destinations", func() {
			BeforeEach(func() {
				route.Spec.Destinations = []korifiv1alpha1.Destination{}
			})

			It("sets empty annotation value", func() {
				Eventually(func(g Gomega) {
					g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(route), route)).To(Succeed())
					g.Expect(route.Annotations).To(MatchKeys(IgnoreExtras, Keys{
						korifiv1alpha1.CFRouteAppGuidsAnnotationKey: BeEmpty(),
					}))
				}).Should(Succeed())
			})
		})

		When("the route is updated to have different destinations", func() {
			var newAppGUID string

			BeforeEach(func() {
				newAppGUID = uuid.NewString()
			})

			JustBeforeEach(func() {
				Expect(k8s.PatchResource(ctx, adminClient, route, func() {
					route.Spec.Destinations = []korifiv1alpha1.Destination{
						{
							AppRef: corev1.LocalObjectReference{
								Name: newAppGUID,
							},
						},
					}
				})).To(Succeed())
			})

			It("updates the annotation", func() {
				Eventually(func(g Gomega) {
					g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(route), route)).To(Succeed())
					g.Expect(route.Annotations).To(MatchKeys(IgnoreExtras, Keys{
						korifiv1alpha1.CFRouteAppGuidsAnnotationKey: Equal(newAppGUID),
					}))
				}).Should(Succeed())
			})
		})

		When("route is updated to have no destinations", func() {
			JustBeforeEach(func() {
				Expect(k8s.PatchResource(ctx, adminClient, route, func() {
					route.Spec.Destinations = []korifiv1alpha1.Destination{}
				})).To(Succeed())
			})

			It("sets empty value to the annotation", func() {
				Eventually(func(g Gomega) {
					g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(route), route)).To(Succeed())
					g.Expect(route.Annotations).To(MatchKeys(IgnoreExtras, Keys{
						korifiv1alpha1.CFRouteAppGuidsAnnotationKey: BeEmpty(),
					}))
				}).Should(Succeed())
			})
		})
	})
})
