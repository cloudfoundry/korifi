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
)

var _ = Describe("CFTask Webhook", func() {
	var (
		cftask      v1alpha1.CFTask
		creationErr error
	)

	BeforeEach(func() {
		cftask = v1alpha1.CFTask{
			ObjectMeta: metav1.ObjectMeta{
				Name:      testutils.GenerateGUID(),
				Namespace: rootNamespace,
			},
			Spec: v1alpha1.CFTaskSpec{
				Command: []string{"echo", "hello"},
				AppRef: corev1.LocalObjectReference{
					Name: "cfapp",
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
})
