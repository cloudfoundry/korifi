package ports_test

import (
	"time"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/controllers/workloads/ports"
	"code.cloudfoundry.org/korifi/tools"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("FromRoutes", func() {
	var (
		cfRoutes     []korifiv1alpha1.CFRoute
		processPorts []int32
	)

	BeforeEach(func() {
		cfRoutes = []korifiv1alpha1.CFRoute{{
			ObjectMeta: metav1.ObjectMeta{
				CreationTimestamp: metav1.NewTime(time.UnixMilli(2000)),
			},
			Status: korifiv1alpha1.CFRouteStatus{
				Destinations: []korifiv1alpha1.Destination{{
					GUID: "dest-guid",
					Port: tools.PtrTo(9876),
					AppRef: corev1.LocalObjectReference{
						Name: "my-app",
					},
					ProcessType: "web",
				}},
			},
		}}
	})

	JustBeforeEach(func() {
		processPorts = ports.FromRoutes(cfRoutes, "my-app", "web")
	})

	It("returns the ports for the given routes", func() {
		Expect(processPorts).To(ConsistOf(int32(9876)))
	})

	When("there are multiple routes to the process", func() {
		BeforeEach(func() {
			cfRoutes = append(cfRoutes, korifiv1alpha1.CFRoute{
				ObjectMeta: metav1.ObjectMeta{
					CreationTimestamp: metav1.NewTime(time.UnixMilli(1000)),
				},
				Status: korifiv1alpha1.CFRouteStatus{
					Destinations: []korifiv1alpha1.Destination{{
						GUID: "dest-guid",
						Port: tools.PtrTo(1234),
						AppRef: corev1.LocalObjectReference{
							Name: "my-app",
						},
						ProcessType: "web",
					}},
				},
			})
		})

		It("returns the ports ordered by route age", func() {
			Expect(processPorts).To(Equal([]int32{1234, 9876}))
		})
	})

	When("the destination does not reference the process app", func() {
		BeforeEach(func() {
			cfRoutes[0].Status.Destinations[0].AppRef.Name = "foo"
		})

		It("returns an empty list", func() {
			Expect(processPorts).To(BeEmpty())
		})
	})

	When("the destination type does not match the process type", func() {
		BeforeEach(func() {
			cfRoutes[0].Status.Destinations[0].ProcessType = "another-type"
		})

		It("returns an empty list", func() {
			Expect(processPorts).To(BeEmpty())
		})
	})

	When("the destination has no port set", func() {
		BeforeEach(func() {
			cfRoutes[0].Status.Destinations[0].Port = nil
		})

		It("returns an empty list", func() {
			Expect(processPorts).To(BeEmpty())
		})
	})

	When("the route does not have destinations", func() {
		BeforeEach(func() {
			cfRoutes[0].Status.Destinations = []korifiv1alpha1.Destination{}
		})

		It("returns an empty list", func() {
			Expect(processPorts).To(BeEmpty())
		})
	})
})
