package integration_test

import (
	"context"

	"code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/controllers/workloads/testutils"
	"code.cloudfoundry.org/korifi/controllers/webhooks"
	"code.cloudfoundry.org/korifi/controllers/webhooks/workloads"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("CFTask Webhook", func() {
	var (
		cftask      v1alpha1.CFTask
		creationErr error
	)

	BeforeEach(func() {
		cfApp := makeCFApp(testutils.PrefixedGUID("cfapp"), rootNamespace, testutils.PrefixedGUID("appName"))
		Expect(k8sClient.Create(context.Background(), cfApp)).To(Succeed())

		cftask = v1alpha1.CFTask{
			ObjectMeta: metav1.ObjectMeta{
				Name:      testutils.GenerateGUID(),
				Namespace: rootNamespace,
			},
			Spec: v1alpha1.CFTaskSpec{
				Command: []string{"echo", "hello"},
				AppRef: corev1.LocalObjectReference{
					Name: cfApp.Name,
				},
			},
		}
	})

	JustBeforeEach(func() {
		creationErr = k8sClient.Create(context.Background(), &cftask)
	})

	It("suceeds", func() {
		Expect(creationErr).NotTo(HaveOccurred())
	})

	When("command is missing", func() {
		BeforeEach(func() {
			cftask.Spec.Command = nil
		})

		It("returns a validation error", func() {
			validationErr, ok := webhooks.WebhookErrorToValidationError(creationErr)
			Expect(ok).To(BeTrue())

			Expect(validationErr.Type).To(Equal(workloads.MissingRequredFieldErrorType))
			Expect(validationErr.Message).To(ContainSubstring("missing required field 'Spec.Command'"))
		})
	})

	When("the app does not exist", func() {
		BeforeEach(func() {
			cftask.Spec.AppRef.Name = "i-do-not-exist"
		})

		It("records an app missing event", func() {
			eventList := corev1.EventList{}
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.List(context.Background(),
					&eventList,
					client.InNamespace(rootNamespace),
					client.MatchingFields{
						"involvedObject.namespace": cftask.Namespace,
						"involvedObject.name":      cftask.Name,
						"involvedObject.uid":       string(cftask.UID),
					},
				)).To(Succeed())
				g.Expect(eventList.Items).To(HaveLen(1))
			}).Should(Succeed())

			event := eventList.Items[0]
			Expect(event.Type).To(Equal("Warning"))
			Expect(event.Reason).To(Equal("appNotFound"))
			Expect(event.Message).To(ContainSubstring("Did not find app"))
		})
	})
})
