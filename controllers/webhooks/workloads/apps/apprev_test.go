package apps_test

import (
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/tools/k8s"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("ApprevWebhook", func() {
	var app *korifiv1alpha1.CFApp

	BeforeEach(func() {
		app = &korifiv1alpha1.CFApp{
			ObjectMeta: metav1.ObjectMeta{
				Name:      uuid.NewString(),
				Namespace: testNamespace,
				Annotations: map[string]string{
					korifiv1alpha1.CFAppRevisionKey: "5",
				},
			},
			Spec: korifiv1alpha1.CFAppSpec{
				DisplayName:  uuid.NewString(),
				DesiredState: korifiv1alpha1.StoppedState,
				Lifecycle: korifiv1alpha1.Lifecycle{
					Type: "buildpack",
				},
			},
		}
		Expect(adminClient.Create(ctx, app)).To(Succeed())

		Expect(k8s.Patch(ctx, adminClient, app, func() {
			app.Spec.DesiredState = korifiv1alpha1.StartedState
		})).To(Succeed())
	})

	When("the app change does not affect the app rev", func() {
		JustBeforeEach(func() {
			Expect(k8s.Patch(ctx, adminClient, app, func() {
				app.Spec.DisplayName = "changed-display-name"
			})).To(Succeed())
		})

		It("does not change the app rev", func() {
			Expect(app.Annotations[korifiv1alpha1.CFAppRevisionKey]).To(Equal("5"))
		})
	})

	When("desiredState changes from started to stopped", func() {
		JustBeforeEach(func() {
			Expect(k8s.Patch(ctx, adminClient, app, func() {
				app.Spec.DesiredState = korifiv1alpha1.StoppedState
			})).To(Succeed())
		})

		It("increments the app rev", func() {
			Expect(app.Annotations[korifiv1alpha1.CFAppRevisionKey]).To(Equal("6"))
		})

		It("updates status.lastStopAppRev", func() {
			Expect(app.Annotations[korifiv1alpha1.CFAppLastStopRevisionKey]).To(Equal("6"))
		})

		When("the app rev is not a number", func() {
			BeforeEach(func() {
				Expect(k8s.Patch(ctx, adminClient, app, func() {
					app.Annotations[korifiv1alpha1.CFAppRevisionKey] = "a"
				})).To(Succeed())
			})

			It("defaults the app rev to 0", func() {
				Expect(app.Annotations[korifiv1alpha1.CFAppRevisionKey]).To(Equal("0"))
			})
		})

		When("the app rev is negative", func() {
			BeforeEach(func() {
				Expect(k8s.Patch(ctx, adminClient, app, func() {
					app.Annotations[korifiv1alpha1.CFAppRevisionKey] = "-10"
				})).To(Succeed())
			})

			It("sets the app rev to 0", func() {
				Expect(app.Annotations[korifiv1alpha1.CFAppRevisionKey]).To(Equal("0"))
			})
		})
	})
})
