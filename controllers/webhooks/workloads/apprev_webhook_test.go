package workloads_test

import (
	"context"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("ApprevWebhook", func() {
	var (
		ctx         context.Context
		namespace   string
		app         *korifiv1alpha1.CFApp
		originalApp *korifiv1alpha1.CFApp
	)

	BeforeEach(func() {
		ctx = context.Background()

		namespace = "ns-" + uuid.NewString()

		Expect(k8sClient.Create(ctx, &v1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: namespace,
			},
		})).To(Succeed())

		app = makeCFApp(uuid.NewString(), namespace, uuid.NewString())
		app.Annotations = map[string]string{
			korifiv1alpha1.CFAppRevisionKey: "5",
		}
		app.Spec.DesiredState = korifiv1alpha1.StartedState
		Expect(k8sClient.Create(ctx, app)).To(Succeed())
		originalApp = app.DeepCopy()
	})

	JustBeforeEach(func() {
		app.Spec.DisplayName = "changed-display-name"
		Expect(k8sClient.Patch(ctx, app, client.MergeFrom(originalApp))).To(Succeed())
	})

	It("does not change the app rev", func() {
		Expect(app.Annotations[korifiv1alpha1.CFAppRevisionKey]).To(Equal("5"))
	})

	When("desiredState changes from started to stopped", func() {
		BeforeEach(func() {
			app.Spec.DesiredState = korifiv1alpha1.StoppedState
		})

		It("increments the app rev", func() {
			Expect(app.Annotations[korifiv1alpha1.CFAppRevisionKey]).To(Equal("6"))
		})

		When("the app rev is not a number", func() {
			BeforeEach(func() {
				app.Annotations[korifiv1alpha1.CFAppRevisionKey] = "a"
				Expect(k8sClient.Patch(ctx, app, client.MergeFrom(originalApp))).To(Succeed())
				originalApp = app.DeepCopy()
			})

			It("defaults the app rev to 0", func() {
				Expect(app.Annotations[korifiv1alpha1.CFAppRevisionKey]).To(Equal("0"))
			})
		})

		When("the app rev is negative", func() {
			BeforeEach(func() {
				app.Annotations[korifiv1alpha1.CFAppRevisionKey] = "-10"
				Expect(k8sClient.Patch(ctx, app, client.MergeFrom(originalApp))).To(Succeed())
				originalApp = app.DeepCopy()
			})

			It("sets the app rev to 0", func() {
				Expect(app.Annotations[korifiv1alpha1.CFAppRevisionKey]).To(Equal("0"))
			})
		})
	})
})
