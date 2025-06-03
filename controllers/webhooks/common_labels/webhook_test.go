package common_labels_test

import (
	"time"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/tools/k8s"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("CommonLabelsWebhook", func() {
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
		Expect(adminClient.Create(ctx, route)).To(Succeed())
	})

	It("sets the created_at label", func() {
		Eventually(func(g Gomega) {
			g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(route), route)).To(Succeed())
			g.Expect(route.Labels).To(MatchKeys(IgnoreExtras, Keys{
				korifiv1alpha1.CreatedAtLabelKey: Equal(route.CreationTimestamp.Format(korifiv1alpha1.LabelDateFormat)),
			}))
		}).Should(Succeed())
	})

	It("does not set the updated_at label", func() {
		Consistently(func(g Gomega) {
			g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(route), route)).To(Succeed())
			g.Expect(route.Labels).NotTo(HaveKey(korifiv1alpha1.UpdatedAtLabelKey))
		}).Should(Succeed())
	})

	When("the object is updated", func() {
		BeforeEach(func() {
			time.Sleep(1100 * time.Millisecond)
			Expect(k8s.PatchResource(ctx, adminClient, route, func() {
				route.Labels["foo"] = "bar"
			})).To(Succeed())
		})

		It("sets the updated_at label", func() {
			Eventually(func(g Gomega) {
				g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(route), route)).To(Succeed())
				g.Expect(route.Labels).To(HaveKey(korifiv1alpha1.UpdatedAtLabelKey))

				updatedAt, err := time.Parse(korifiv1alpha1.LabelDateFormat, route.Labels[korifiv1alpha1.UpdatedAtLabelKey])
				Expect(err).NotTo(HaveOccurred())
				g.Expect(updatedAt).To(BeTemporally(">", route.CreationTimestamp.Time))
			}).Should(Succeed())
		})
	})
})
