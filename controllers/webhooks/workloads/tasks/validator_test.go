package tasks_test

import (
	"context"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/webhooks"
	"code.cloudfoundry.org/korifi/controllers/webhooks/validation"
	"code.cloudfoundry.org/korifi/controllers/webhooks/workloads/tasks"
	"code.cloudfoundry.org/korifi/tools/k8s"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("CFTask Validator", func() {
	var (
		cfTask      *korifiv1alpha1.CFTask
		creationErr error
	)

	BeforeEach(func() {
		cfTask = &korifiv1alpha1.CFTask{
			ObjectMeta: metav1.ObjectMeta{
				Name:      uuid.NewString(),
				Namespace: testNamespace,
			},
			Spec: korifiv1alpha1.CFTaskSpec{
				Command: "echo hello",
				AppRef: corev1.LocalObjectReference{
					Name: uuid.NewString(),
				},
			},
		}
	})

	Describe("create", func() {
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
				validationErr, ok := validation.WebhookErrorToValidationError(creationErr)
				Expect(ok).To(BeTrue())

				Expect(validationErr.Type).To(Equal(webhooks.MissingRequredFieldErrorType))
				Expect(validationErr.Message).To(ContainSubstring("missing required field 'Spec.Command'"))
			})
		})

		When("the app reference is not set", func() {
			BeforeEach(func() {
				cfTask.Spec.AppRef = corev1.LocalObjectReference{}
			})

			It("returns a validation error", func() {
				validationErr, ok := validation.WebhookErrorToValidationError(creationErr)
				Expect(ok).To(BeTrue())

				Expect(validationErr.Type).To(Equal(webhooks.MissingRequredFieldErrorType))
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

				creationErr = k8s.Patch(ctx, adminClient, cfTask, func() {
					cfTask.Status.SequenceID = seqId
				})
			})

			It("suceeds", func() {
				Expect(creationErr).NotTo(HaveOccurred())
			})

			When("the sequenceID is set to a negative value", func() {
				BeforeEach(func() {
					seqId = -1
				})

				It("returns a validation error", func() {
					validationErr, ok := validation.WebhookErrorToValidationError(creationErr)
					Expect(ok).To(BeTrue())

					Expect(validationErr.Type).To(Equal(webhooks.InvalidFieldValueErrorType))
					Expect(validationErr.Message).To(ContainSubstring("SequenceID cannot be negative"))
				})
			})
		})
	})

	Describe("update", func() {
		var (
			updateErr  error
			updateFunc func()
		)

		BeforeEach(func() {
			Expect(adminClient.Create(ctx, cfTask)).To(Succeed())
			Expect(k8s.Patch(ctx, adminClient, cfTask, func() {
				cfTask.Status = korifiv1alpha1.CFTaskStatus{}
			})).To(Succeed())

			updateFunc = func() {}
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
					validationErr, ok := validation.WebhookErrorToValidationError(updateErr)
					Expect(ok).To(BeTrue())

					Expect(validationErr.Type).To(Equal(tasks.CancelationNotPossibleErrorType))
					Expect(validationErr.Message).To(ContainSubstring("cannot be canceled"))
				})
			})

			When("the cftask has a failed condition", func() {
				BeforeEach(func() {
					setStatusCondition(cfTask, korifiv1alpha1.TaskFailedConditionType)
				})

				It("fails", func() {
					Expect(updateErr).To(HaveOccurred())
					validationErr, ok := validation.WebhookErrorToValidationError(updateErr)
					Expect(ok).To(BeTrue())

					Expect(validationErr.Type).To(Equal(tasks.CancelationNotPossibleErrorType))
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
				validationErr, ok := validation.WebhookErrorToValidationError(updateErr)
				Expect(ok).To(BeTrue())

				Expect(validationErr.Type).To(Equal(webhooks.ImmutableFieldModificationErrorType))
				Expect(validationErr.Message).To(ContainSubstring("SequenceID is immutable"))
			})
		})
	})
})

func setStatusCondition(cftask *korifiv1alpha1.CFTask, conditionType string) {
	GinkgoHelper()

	Expect(k8s.Patch(ctx, adminClient, cftask, func() {
		meta.SetStatusCondition(&cftask.Status.Conditions, metav1.Condition{
			Type:    conditionType,
			Status:  metav1.ConditionTrue,
			Reason:  "foo",
			Message: "bar",
		})
	})).To(Succeed())
}
