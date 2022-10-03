package integration_test

import (
	"context"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/controllers/workloads/testutils"
	"code.cloudfoundry.org/korifi/controllers/webhooks"
	"code.cloudfoundry.org/korifi/controllers/webhooks/workloads"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("CFTask Creation", func() {
	var (
		cftask      korifiv1alpha1.CFTask
		creationErr error
	)

	BeforeEach(func() {
		cfApp := makeCFApp(testutils.PrefixedGUID("cfapp"), rootNamespace, testutils.PrefixedGUID("appName"))
		Expect(k8sClient.Create(context.Background(), cfApp)).To(Succeed())

		cftask = korifiv1alpha1.CFTask{
			ObjectMeta: metav1.ObjectMeta{
				Name:      testutils.GenerateGUID(),
				Namespace: rootNamespace,
			},
			Spec: korifiv1alpha1.CFTaskSpec{
				Command: "echo hello",
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
			cftask.Spec.Command = ""
		})

		It("returns a validation error", func() {
			validationErr, ok := webhooks.WebhookErrorToValidationError(creationErr)
			Expect(ok).To(BeTrue())

			Expect(validationErr.Type).To(Equal(workloads.MissingRequredFieldErrorType))
			Expect(validationErr.Message).To(ContainSubstring("missing required field 'Spec.Command'"))
		})
	})

	When("the app reference is not set", func() {
		BeforeEach(func() {
			cftask.Spec.AppRef = corev1.LocalObjectReference{}
		})

		It("returns a validation error", func() {
			validationErr, ok := webhooks.WebhookErrorToValidationError(creationErr)
			Expect(ok).To(BeTrue())

			Expect(validationErr.Type).To(Equal(workloads.MissingRequredFieldErrorType))
			Expect(validationErr.Message).To(ContainSubstring("missing required field 'Spec.AppRef.Name'"))
		})
	})
})

var _ = Describe("CFTask Update", func() {
	var (
		cftask         *korifiv1alpha1.CFTask
		originalCFTask *korifiv1alpha1.CFTask
		updateErr      error
	)

	BeforeEach(func() {
		cfApp := makeCFApp(testutils.PrefixedGUID("cfapp"), rootNamespace, testutils.PrefixedGUID("appName"))
		Expect(k8sClient.Create(context.Background(), cfApp)).To(Succeed())

		cftask = &korifiv1alpha1.CFTask{
			ObjectMeta: metav1.ObjectMeta{
				Name:      testutils.GenerateGUID(),
				Namespace: rootNamespace,
			},
			Spec: korifiv1alpha1.CFTaskSpec{
				Command: "echo hello",
				AppRef: corev1.LocalObjectReference{
					Name: cfApp.Name,
				},
			},
		}
		Expect(k8sClient.Create(context.Background(), cftask)).To(Succeed())
		originalCFTask = cftask.DeepCopy()
	})

	JustBeforeEach(func() {
		updateErr = k8sClient.Patch(context.Background(), cftask, client.MergeFrom(originalCFTask))
	})

	When("canceled is not changed", func() {
		BeforeEach(func() {
			cftask.Spec.Command = "echo ok"
		})

		It("succeeds", func() {
			Expect(updateErr).NotTo(HaveOccurred())
		})
	})

	When("the task gets canceled", func() {
		BeforeEach(func() {
			cftask.Spec.Canceled = true
		})

		It("succeeds", func() {
			Expect(updateErr).NotTo(HaveOccurred())
		})

		When("the cftask has a succeeded contdition", func() {
			BeforeEach(func() {
				setStatusCondition(cftask, korifiv1alpha1.TaskSucceededConditionType)
			})

			It("fails", func() {
				Expect(updateErr).To(HaveOccurred())
				validationErr, ok := webhooks.WebhookErrorToValidationError(updateErr)
				Expect(ok).To(BeTrue())

				Expect(validationErr.Type).To(Equal(workloads.CancelationNotPossibleErrorType))
				Expect(validationErr.Message).To(ContainSubstring("cannot be canceled"))
			})
		})

		When("the cftask has a failed contdition", func() {
			BeforeEach(func() {
				setStatusCondition(cftask, korifiv1alpha1.TaskFailedConditionType)
			})

			It("fails", func() {
				Expect(updateErr).To(HaveOccurred())
				validationErr, ok := webhooks.WebhookErrorToValidationError(updateErr)
				Expect(ok).To(BeTrue())

				Expect(validationErr.Type).To(Equal(workloads.CancelationNotPossibleErrorType))
				Expect(validationErr.Message).To(ContainSubstring("cannot be canceled"))
			})
		})

		When("the task is already canceled before an update", func() {
			BeforeEach(func() {
				Expect(k8sClient.Patch(context.Background(), cftask, client.MergeFrom(originalCFTask))).To(Succeed())
				originalCFTask = cftask.DeepCopy()
				setStatusCondition(cftask, korifiv1alpha1.TaskSucceededConditionType)
				cftask.Spec.Command = "echo foo"
			})

			It("succeeds", func() {
				Expect(updateErr).NotTo(HaveOccurred())
			})
		})
	})
})

func setStatusCondition(cftask *korifiv1alpha1.CFTask, conditionType string) {
	clone := cftask.DeepCopy()
	meta.SetStatusCondition(&cftask.Status.Conditions, metav1.Condition{
		Type:    conditionType,
		Status:  metav1.ConditionTrue,
		Reason:  "foo",
		Message: "bar",
	})
	Expect(k8sClient.Status().Patch(context.Background(), cftask, client.MergeFrom(clone))).To(Succeed())

	// the status update clears any unapplied changes to the rest of the object, so reset spec changes:
	cftask.Spec = clone.Spec
}
