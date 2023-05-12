package workloads_test

import (
	"time"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/tools/k8s"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("StartedAtWebhook", func() {
	var (
		namespace string
		app       *korifiv1alpha1.CFApp
	)

	BeforeEach(func() {
		namespace = "ns-" + uuid.NewString()

		Expect(k8sClient.Create(ctx, &v1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: namespace,
			},
		})).To(Succeed())

		app = makeCFApp(uuid.NewString(), namespace, uuid.NewString())
	})

	JustBeforeEach(func() {
		Expect(k8sClient.Create(ctx, app)).To(Succeed())

		Expect(k8s.Patch(ctx, k8sClient, app, func() {
			app.Labels = map[string]string{"foo": "bar"}
		})).To(Succeed())
	})

	It("does not set the startedAt annotation", func() {
		Expect(app.Annotations[korifiv1alpha1.StartedAtAnnotation]).To(BeEmpty())
	})

	When("desiredState changes from stopped to started", func() {
		JustBeforeEach(func() {
			Expect(k8s.Patch(ctx, k8sClient, app, func() {
				app.Spec.DesiredState = korifiv1alpha1.StartedState
			})).To(Succeed())
		})

		It("sets the startedAt annotation to a valid timestamp", func() {
			startedAt := app.Annotations[korifiv1alpha1.StartedAtAnnotation]
			Expect(startedAt).NotTo(BeEmpty())

			_, err := time.Parse(time.RFC3339, startedAt)
			Expect(err).NotTo(HaveOccurred())
		})

		When("the startedAt annotation is already set", func() {
			var firstStartTime time.Time

			BeforeEach(func() {
				firstStartTime = time.Now().Add(-5 * time.Second)
				app.Annotations = map[string]string{
					korifiv1alpha1.StartedAtAnnotation: firstStartTime.Format(time.RFC3339),
				}
			})

			It("sets the startedAt annotation to a later timestamp", func() {
				startedAt := app.Annotations[korifiv1alpha1.StartedAtAnnotation]
				Expect(startedAt).NotTo(BeEmpty())

				startTime, err := time.Parse(time.RFC3339, startedAt)
				Expect(err).NotTo(HaveOccurred())

				Expect(startTime).To(BeTemporally(">", firstStartTime))
			})
		})
	})
})
