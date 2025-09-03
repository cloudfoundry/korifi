package repositories_test

import (
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"code.cloudfoundry.org/korifi/api/repositories"
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
)

var _ = Describe("FilterRoutesByAppGUID", func() {
	var (
		appGUID     string
		appGUID1    string
		cfRoute     *korifiv1alpha1.CFRoute
		cfRouteList *korifiv1alpha1.CFRouteList
		routes      []korifiv1alpha1.CFRoute
		appGUIDs    []string
	)

	BeforeEach(func() {
		appGUID = uuid.NewString()
		appGUID1 = uuid.NewString()
		cfRoute = &korifiv1alpha1.CFRoute{
			ObjectMeta: metav1.ObjectMeta{
				Name: uuid.NewString(),
			},
			Spec: korifiv1alpha1.CFRouteSpec{
				Destinations: []korifiv1alpha1.Destination{
					{
						AppRef: corev1.LocalObjectReference{
							Name: appGUID,
						},
					},
				},
			},
		}

		cfRouteList = &korifiv1alpha1.CFRouteList{
			Items: []korifiv1alpha1.CFRoute{*cfRoute},
		}
		appGUIDs = []string{appGUID}
	})

	JustBeforeEach(func() {
		routes = repositories.FilterRoutesByAppGUID(cfRouteList, appGUIDs)
	})

	It("returns the route when filtering by an app guid", func() {
		Expect(routes).To(HaveLen(1))
		Expect(routes[0].Name).To(Equal(cfRoute.Name))
	})

	When("route has more than 1 app destination", func() {
		BeforeEach(func() {
			cfRoute.Spec.Destinations = append(cfRoute.Spec.Destinations, korifiv1alpha1.Destination{
				AppRef: corev1.LocalObjectReference{
					Name: appGUID1,
				},
			})

			cfRouteList.Items = []korifiv1alpha1.CFRoute{*cfRoute}
			appGUIDs = append(appGUIDs, appGUID1)
		})

		It("returns the route when filtering by an app guid", func() {
			Expect(routes).To(HaveLen(1))
			Expect(routes[0].Name).To(Equal(cfRoute.Name))
		})
	})

	When("there is more than 1 route with appGUIDs", func() {
		var cfRoute1 *korifiv1alpha1.CFRoute
		BeforeEach(func() {
			cfRoute1 = &korifiv1alpha1.CFRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name: uuid.NewString(),
				},
				Spec: korifiv1alpha1.CFRouteSpec{
					Destinations: []korifiv1alpha1.Destination{
						{
							AppRef: corev1.LocalObjectReference{
								Name: appGUID1,
							},
						},
					},
				},
			}

			cfRouteList.Items = []korifiv1alpha1.CFRoute{*cfRoute, *cfRoute1}
			appGUIDs = append(appGUIDs, appGUID1)
		})

		It("returns both routes when filtering by both app guids", func() {
			Expect(routes).To(HaveLen(2))
			Expect(routes[0].Name).To(Equal(cfRoute.Name))
			Expect(routes[1].Name).To(Equal(cfRoute1.Name))
		})

		When("route has more than 1 app guid", func() {
			BeforeEach(func() {
				appGUID3 := uuid.NewString()
				cfRoute1.Spec.Destinations = append(cfRoute1.Spec.Destinations, korifiv1alpha1.Destination{
					AppRef: corev1.LocalObjectReference{
						Name: appGUID3,
					},
				})

				cfRouteList.Items = []korifiv1alpha1.CFRoute{*cfRoute, *cfRoute1}
			})

			It("returns both routes when filtering by all 3 app guids", func() {
				Expect(routes).To(HaveLen(2))
				Expect(routes[0].Name).To(Equal(cfRoute.Name))
				Expect(routes[1].Name).To(Equal(cfRoute1.Name))
			})
		})

		When("route has no destiantions", func() {
			BeforeEach(func() {
				cfRoute1.Spec.Destinations = []korifiv1alpha1.Destination{}
				cfRouteList.Items = []korifiv1alpha1.CFRoute{*cfRoute, *cfRoute1}
			})

			It("returns only the route with destinations", func() {
				Expect(routes).To(HaveLen(1))
				Expect(routes[0].Name).To(Equal(cfRoute.Name))
			})
		})

		When("there is no app guids passed", func() {
			BeforeEach(func() {
				appGUIDs = []string{}
			})

			It("returns all routes", func() {
				Expect(routes).To(HaveLen(2))
				Expect(routes[0].Name).To(Equal(cfRoute.Name))
				Expect(routes[1].Name).To(Equal(cfRoute1.Name))
			})
		})
	})
})
