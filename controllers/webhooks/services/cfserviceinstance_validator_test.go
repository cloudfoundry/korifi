package services_test

import (
	"context"
	"time"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/webhooks"
	"code.cloudfoundry.org/korifi/controllers/webhooks/fake"
	"code.cloudfoundry.org/korifi/controllers/webhooks/services"
	"code.cloudfoundry.org/korifi/tests/matchers"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

var _ = Describe("CFServiceInstanceValidatingWebhook", func() {
	const (
		defaultNamespace = "default"
	)

	var (
		serviceInstanceGUID string
		serviceInstanceName string
		ctx                 context.Context
		duplicateValidator  *fake.NameValidator
		serviceInstance     *korifiv1alpha1.CFServiceInstance
		validatingWebhook   *services.CFServiceInstanceValidator
		retErr              error
	)

	BeforeEach(func() {
		ctx = context.Background()

		scheme := runtime.NewScheme()
		err := korifiv1alpha1.AddToScheme(scheme)
		Expect(err).NotTo(HaveOccurred())

		serviceInstanceName = generateGUID("service-instance")
		serviceInstanceGUID = generateGUID("service-instance")
		serviceInstance = &korifiv1alpha1.CFServiceInstance{
			ObjectMeta: metav1.ObjectMeta{
				Name:      serviceInstanceGUID,
				Namespace: defaultNamespace,
			},
			Spec: korifiv1alpha1.CFServiceInstanceSpec{
				DisplayName: serviceInstanceName,
				Type:        korifiv1alpha1.UserProvidedType,
			},
		}

		duplicateValidator = new(fake.NameValidator)
		validatingWebhook = services.NewCFServiceInstanceValidator(duplicateValidator)
	})

	Describe("ValidateCreate", func() {
		JustBeforeEach(func() {
			_, retErr = validatingWebhook.ValidateCreate(ctx, serviceInstance)
		})

		It("allows the request", func() {
			Expect(retErr).NotTo(HaveOccurred())
		})

		It("invokes the validator correctly", func() {
			Expect(duplicateValidator.ValidateCreateCallCount()).To(Equal(1))
			actualContext, _, actualNamespace, name, _ := duplicateValidator.ValidateCreateArgsForCall(0)
			Expect(actualContext).To(Equal(ctx))
			Expect(actualNamespace).To(Equal(serviceInstance.Namespace))
			Expect(name).To(Equal(serviceInstance.Spec.DisplayName))
		})

		When("the serviceInstance name is a duplicate", func() {
			BeforeEach(func() {
				duplicateValidator.ValidateCreateReturns(&webhooks.ValidationError{
					Type:    webhooks.DuplicateNameErrorType,
					Message: `The service instance name is taken: ` + serviceInstance.Spec.DisplayName,
				})
			})

			It("denies the request", func() {
				Expect(retErr).To(matchers.BeValidationError(
					webhooks.DuplicateNameErrorType,
					Equal(`The service instance name is taken: `+serviceInstance.Spec.DisplayName),
				))
			})
		})

		When("validating the serviceInstance name fails", func() {
			BeforeEach(func() {
				duplicateValidator.ValidateCreateReturns(&webhooks.ValidationError{
					Type:    webhooks.UnknownErrorType,
					Message: webhooks.UnknownErrorMessage,
				})
			})

			It("denies the request", func() {
				Expect(retErr).To(matchers.BeValidationError(
					webhooks.UnknownErrorType,
					Equal(webhooks.UnknownErrorMessage),
				))
			})
		})
	})

	Describe("ValidateUpdate", func() {
		var updatedServiceInstance *korifiv1alpha1.CFServiceInstance

		BeforeEach(func() {
			updatedServiceInstance = serviceInstance.DeepCopy()
			updatedServiceInstance.Spec.DisplayName = "the-new-name"
		})

		JustBeforeEach(func() {
			_, retErr = validatingWebhook.ValidateUpdate(ctx, serviceInstance, updatedServiceInstance)
		})

		It("allows the request", func() {
			Expect(retErr).NotTo(HaveOccurred())
		})

		It("invokes the validator correctly", func() {
			Expect(duplicateValidator.ValidateUpdateCallCount()).To(Equal(1))
			actualContext, _, actualNamespace, oldName, newName, _ := duplicateValidator.ValidateUpdateArgsForCall(0)
			Expect(actualContext).To(Equal(ctx))
			Expect(actualNamespace).To(Equal(serviceInstance.Namespace))
			Expect(oldName).To(Equal(serviceInstance.Spec.DisplayName))
			Expect(newName).To(Equal(updatedServiceInstance.Spec.DisplayName))
		})

		When("the service instance is being deleted", func() {
			BeforeEach(func() {
				updatedServiceInstance.DeletionTimestamp = &metav1.Time{Time: time.Now()}
			})

			It("does not return an error", func() {
				Expect(retErr).NotTo(HaveOccurred())
			})
		})

		When("the new serviceInstance name is a duplicate", func() {
			BeforeEach(func() {
				duplicateValidator.ValidateUpdateReturns(&webhooks.ValidationError{
					Type:    webhooks.DuplicateNameErrorType,
					Message: `The service instance name is taken: ` + updatedServiceInstance.Spec.DisplayName,
				})
			})

			It("denies the request", func() {
				Expect(retErr).To(matchers.BeValidationError(
					webhooks.DuplicateNameErrorType,
					Equal("The service instance name is taken: "+updatedServiceInstance.Spec.DisplayName),
				))
			})
		})

		When("the update validation fails for another reason", func() {
			BeforeEach(func() {
				duplicateValidator.ValidateUpdateReturns(&webhooks.ValidationError{
					Type:    webhooks.UnknownErrorType,
					Message: webhooks.UnknownErrorMessage,
				})
			})

			It("denies the request", func() {
				Expect(retErr).To(matchers.BeValidationError(
					webhooks.UnknownErrorType,
					Equal(webhooks.UnknownErrorMessage),
				))
			})
		})
	})

	Describe("ValidateDelete", func() {
		JustBeforeEach(func() {
			_, retErr = validatingWebhook.ValidateDelete(ctx, serviceInstance)
		})

		It("allows the request", func() {
			Expect(retErr).NotTo(HaveOccurred())
		})

		It("invokes the validator correctly", func() {
			Expect(duplicateValidator.ValidateDeleteCallCount()).To(Equal(1))
			actualContext, _, actualNamespace, name := duplicateValidator.ValidateDeleteArgsForCall(0)
			Expect(actualContext).To(Equal(ctx))
			Expect(actualNamespace).To(Equal(serviceInstance.Namespace))
			Expect(name).To(Equal(serviceInstance.Spec.DisplayName))
		})

		When("delete validation fails", func() {
			BeforeEach(func() {
				duplicateValidator.ValidateDeleteReturns(&webhooks.ValidationError{
					Type:    webhooks.UnknownErrorType,
					Message: webhooks.UnknownErrorMessage,
				})
			})

			It("disallows the request", func() {
				Expect(retErr).To(matchers.BeValidationError(
					webhooks.UnknownErrorType,
					Equal(webhooks.UnknownErrorMessage),
				))
			})
		})
	})
})
