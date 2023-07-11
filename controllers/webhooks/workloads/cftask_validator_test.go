package workloads_test

import (
	"context"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/controllers/workloads/testutils"
	"code.cloudfoundry.org/korifi/controllers/webhooks"
	"code.cloudfoundry.org/korifi/controllers/webhooks/workloads"
	"code.cloudfoundry.org/korifi/tools/k8s"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("CFTask Creation", func() {
	var (
		cfTask      *korifiv1alpha1.CFTask
		creationErr error
	)

	BeforeEach(func() {
		cfApp := makeCFApp(testutils.PrefixedGUID("cfapp"), rootNamespace, testutils.PrefixedGUID("appName"))
		Expect(adminClient.Create(context.Background(), cfApp)).To(Succeed())

		cfTask = &korifiv1alpha1.CFTask{
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
		creationErr = adminClient.Create(context.Background(), cfTask)
	})

	It("suceeds", func() {
		Expect(creationErr).NotTo(HaveOccurred())
	})

	When("command is missing", func() {
		BeforeEach(func() {
			cfTask.Spec.Command = ""
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
			cfTask.Spec.AppRef = corev1.LocalObjectReference{}
		})

		It("returns a validation error", func() {
			validationErr, ok := webhooks.WebhookErrorToValidationError(creationErr)
			Expect(ok).To(BeTrue())

			Expect(validationErr.Type).To(Equal(workloads.MissingRequredFieldErrorType))
			Expect(validationErr.Message).To(ContainSubstring("missing required field 'Spec.AppRef.Name'"))
		})
	})

	When("the task status is created", func() {
		var seqId int64

		BeforeEach(func() {
			seqId = 0
		})

		JustBeforeEach(func() {
			Expect(creationErr).NotTo(HaveOccurred())

			originalCfTask := cfTask.DeepCopy()
			cfTask.Status = korifiv1alpha1.CFTaskStatus{
				Conditions: []metav1.Condition{},
				SequenceID: seqId,
			}

			creationErr = adminClient.Status().Patch(context.Background(), cfTask, client.MergeFrom(originalCfTask))
		})

		It("suceeds", func() {
			Expect(creationErr).NotTo(HaveOccurred())
		})

		When("the sequenceID is set to a negative value", func() {
			BeforeEach(func() {
				seqId = -1
			})

			It("returns a validation error", func() {
				validationErr, ok := webhooks.WebhookErrorToValidationError(creationErr)
				Expect(ok).To(BeTrue())

				Expect(validationErr.Type).To(Equal(workloads.InvalidFieldValueErrorType))
				Expect(validationErr.Message).To(ContainSubstring("SequenceID cannot be negative"))
			})
		})
	})
})

var _ = Describe("CFTask Update", func() {
	var (
		cfTask     *korifiv1alpha1.CFTask
		updateErr  error
		updateFunc func()
	)

	BeforeEach(func() {
		cfApp := makeCFApp(testutils.PrefixedGUID("cfapp"), rootNamespace, testutils.PrefixedGUID("appName"))
		Expect(adminClient.Create(context.Background(), cfApp)).To(Succeed())
		updateFunc = func() {}

		cfTask = &korifiv1alpha1.CFTask{
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
		Expect(adminClient.Create(context.Background(), cfTask)).To(Succeed())
		Expect(k8s.Patch(context.Background(), adminClient, cfTask, func() {
			cfTask.Status = korifiv1alpha1.CFTaskStatus{
				Conditions: []metav1.Condition{},
			}
		})).To(Succeed())
	})

	JustBeforeEach(func() {
		updateErr = k8s.Patch(context.Background(), adminClient, cfTask, updateFunc)
	})

	When("canceled is not changed", func() {
		BeforeEach(func() {
			updateFunc = func() {
				cfTask.Spec.Command = "echo ok"
			}
		})

		It("succeeds", func() {
			Expect(updateErr).NotTo(HaveOccurred())
		})
	})

	When("the task gets canceled", func() {
		BeforeEach(func() {
			updateFunc = func() {
				cfTask.Spec.Canceled = true
			}
		})

		It("succeeds", func() {
			Expect(updateErr).NotTo(HaveOccurred())
		})

		When("the cftask has a succeeded contdition", func() {
			BeforeEach(func() {
				setStatusCondition(cfTask, korifiv1alpha1.TaskSucceededConditionType)
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
				setStatusCondition(cfTask, korifiv1alpha1.TaskFailedConditionType)
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
				Expect(k8s.Patch(context.Background(), adminClient, cfTask, func() {
					cfTask.Spec.Canceled = true
				})).To(Succeed())

				updateFunc = func() {
					cfTask.Spec.Command = "echo foo"
				}
			})

			It("succeeds", func() {
				Expect(updateErr).NotTo(HaveOccurred())
			})
		})
	})

	When("the task Status.SequenceID is updated", func() {
		BeforeEach(func() {
			updateFunc = func() {
				cfTask.Status.SequenceID = 1
			}
		})

		It("denies the request", func() {
			Expect(updateErr).To(HaveOccurred())
			validationErr, ok := webhooks.WebhookErrorToValidationError(updateErr)
			Expect(ok).To(BeTrue())

			Expect(validationErr.Type).To(Equal(workloads.ImmutableFieldModificationErrorType))
			Expect(validationErr.Message).To(ContainSubstring("SequenceID is immutable"))
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
	Expect(adminClient.Status().Patch(context.Background(), cftask, client.MergeFrom(clone))).To(Succeed())

	// the status update clears any unapplied changes to the rest of the object, so reset spec changes:
	cftask.Spec = clone.Spec
}
