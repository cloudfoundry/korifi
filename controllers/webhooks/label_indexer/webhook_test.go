package label_indexer_test

import (
	"maps"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("LabelIndexerWebhook", func() {
	Describe("CFRoute", func() {
		var route *korifiv1alpha1.CFRoute

		BeforeEach(func() {
			route = &korifiv1alpha1.CFRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      uuid.NewString(),
					Namespace: namespace,
				},
				Spec: korifiv1alpha1.CFRouteSpec{
					Host: "example",
					Path: "/example",
					DomainRef: corev1.ObjectReference{
						Name: "example.com",
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
				g.Expect(route.Labels).To(MatchKeys(IgnoreExtras, Keys{
					korifiv1alpha1.CFDomainGUIDLabelKey:      Equal("example.com"),
					korifiv1alpha1.SpaceGUIDKey:              Equal(namespace),
					korifiv1alpha1.CFRouteIsUnmappedLabelKey: Equal("true"),
					korifiv1alpha1.CFRouteHostLabelKey:       Equal("example"),
					korifiv1alpha1.CFRoutePathLabelKey:       Equal("4757e8253d1d2e04aa277d3b9178cf69d8383d43fd9f894f9460ebda"), // SHA224 hash of "/example"
				}))
			}).Should(Succeed())
		})

		When("the route has destinations", func() {
			var app1GUID, app2GUID string

			BeforeEach(func() {
				app1GUID = uuid.NewString()
				app2GUID = uuid.NewString()

				route.Spec.Destinations = []korifiv1alpha1.Destination{
					{AppRef: corev1.LocalObjectReference{Name: app1GUID}},
					{AppRef: corev1.LocalObjectReference{Name: app2GUID}},
				}
			})

			It("adds labels per every destination app", func() {
				Eventually(func(g Gomega) {
					g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(route), route)).To(Succeed())
					g.Expect(route.Labels).To(MatchKeys(IgnoreExtras, Keys{
						korifiv1alpha1.DestinationAppGUIDLabelPrefix + app1GUID: BeEmpty(),
						korifiv1alpha1.DestinationAppGUIDLabelPrefix + app2GUID: BeEmpty(),
						korifiv1alpha1.CFRouteIsUnmappedLabelKey:                Equal("false"),
					}))
				}).Should(Succeed())
			})

			When("the destination does not have app ref", func() {
				BeforeEach(func() {
					route.Spec.Destinations = []korifiv1alpha1.Destination{
						{GUID: "dest-guid"},
					}
				})

				It("does not add destination app labels", func() {
					Consistently(func(g Gomega) {
						g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(route), route)).To(Succeed())
						g.Expect(maps.Keys(route.Labels)).NotTo(ContainElement(HavePrefix(korifiv1alpha1.DestinationAppGUIDLabelPrefix)))
					}).Should(Succeed())
				})
			})
		})
	})
})
