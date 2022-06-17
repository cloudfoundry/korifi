package workloads_test

import (
	"context"
	"fmt"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	controllersfake "code.cloudfoundry.org/korifi/controllers/fake"
	"code.cloudfoundry.org/korifi/controllers/webhooks"
	"code.cloudfoundry.org/korifi/controllers/webhooks/workloads"
	"code.cloudfoundry.org/korifi/controllers/webhooks/workloads/fake"
	"code.cloudfoundry.org/korifi/tests/matchers"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

var _ = Describe("CFTaskValidator", func() {
	const (
		testTaskGUID      = "test-task-guid"
		testTaskNamespace = "default"
		appGUID           = "test-app-guid"
	)

	var (
		ctx                context.Context
		cfTask             *korifiv1alpha1.CFTask
		appExistsValidator *fake.CFAppExistsValidator
		recorder           *controllersfake.EventRecorder
		validatingWebhook  *workloads.CFTaskValidator
		retErr             error
	)

	BeforeEach(func() {
		ctx = context.Background()
		appExistsValidator = new(fake.CFAppExistsValidator)
		recorder = new(controllersfake.EventRecorder)

		scheme := runtime.NewScheme()
		err := korifiv1alpha1.AddToScheme(scheme)
		Expect(err).NotTo(HaveOccurred())

		cfTask = &korifiv1alpha1.CFTask{
			ObjectMeta: metav1.ObjectMeta{
				Name:      testTaskGUID,
				Namespace: testTaskNamespace,
			},
			Spec: korifiv1alpha1.CFTaskSpec{
				AppRef: v1.LocalObjectReference{
					Name: appGUID,
				},
				Command: []string{"some-command"},
			},
		}

		validatingWebhook = workloads.NewCFTaskValidator(appExistsValidator, recorder)
	})

	Describe("ValidateCreate", func() {
		JustBeforeEach(func() {
			retErr = validatingWebhook.ValidateCreate(ctx, cfTask)
		})

		It("allows the request", func() {
			Expect(retErr).NotTo(HaveOccurred())
		})

		It("ensures that the app exists", func() {
			Expect(appExistsValidator.EnsureCFAppCallCount()).To(Equal(1))
			_, actualNamespace, actualAppGUID := appExistsValidator.EnsureCFAppArgsForCall(0)
			Expect(actualNamespace).To(Equal(testTaskNamespace))
			Expect(actualAppGUID).To(Equal(appGUID))
		})

		When("the app ref is not set", func() {
			BeforeEach(func() {
				cfTask.Spec.AppRef = v1.LocalObjectReference{}
			})

			It("denies the request", func() {
				Expect(retErr).To(matchers.RepresentJSONifiedValidationError(webhooks.ValidationError{
					Type:    workloads.MissingRequredFieldErrorType,
					Message: fmt.Sprintf("task %s:%s is missing required field 'Spec.AppRef.Name'", testTaskNamespace, testTaskGUID),
				}))
			})
		})

		When("the task references an app that does not exist", func() {
			BeforeEach(func() {
				appExistsValidator.EnsureCFAppReturns(&webhooks.ValidationError{
					Type:    webhooks.AppDoesNotExistErrorType,
					Message: "whatevs",
				})
			})

			It("allows the request", func() {
				Expect(retErr).NotTo(HaveOccurred())
			})

			It("records a warning event that the app does not exist", func() {
				Expect(recorder.EventfCallCount()).To(Equal(1))

				actualObject, actualEventType, actualReason, actualMsgFmt, actualArgs := recorder.EventfArgsForCall(0)
				Expect(actualObject).To(Equal(cfTask))
				Expect(actualEventType).To(Equal("Warning"))
				Expect(actualReason).To(Equal("appNotFound"))
				Expect(actualMsgFmt).To(Equal("Did not find app with name %s in namespace %s"))
				Expect(actualArgs).To(ConsistOf(appGUID, testTaskNamespace))
			})
		})

		When("ensuring app existence fails", func() {
			BeforeEach(func() {
				appExistsValidator.EnsureCFAppReturns(&webhooks.ValidationError{
					Type:    "foo",
					Message: "bar",
				})
			})

			It("denies the request", func() {
				Expect(retErr).To(matchers.RepresentJSONifiedValidationError(webhooks.ValidationError{
					Type:    "foo",
					Message: "bar",
				}))
			})
		})

		When("the list of commands is empty", func() {
			BeforeEach(func() {
				cfTask.Spec.Command = []string{}
			})

			It("denies the request", func() {
				Expect(retErr).To(matchers.RepresentJSONifiedValidationError(webhooks.ValidationError{
					Type:    workloads.MissingRequredFieldErrorType,
					Message: "task " + testTaskNamespace + ":" + testTaskGUID + " is missing required field 'Spec.Command'",
				}))
			})
		})
	})

	Describe("ValidateUpdate", func() {
		var updatedCFTask *korifiv1alpha1.CFTask

		BeforeEach(func() {
			updatedCFTask = cfTask.DeepCopy()
		})

		JustBeforeEach(func() {
			retErr = validatingWebhook.ValidateUpdate(ctx, cfTask, updatedCFTask)
		})

		It("allows the request", func() {
			Expect(retErr).NotTo(HaveOccurred())
		})
	})

	Describe("ValidateDelete", func() {
		JustBeforeEach(func() {
			retErr = validatingWebhook.ValidateDelete(ctx, cfTask)
		})

		It("allows the request", func() {
			Expect(retErr).NotTo(HaveOccurred())
		})
	})
})
