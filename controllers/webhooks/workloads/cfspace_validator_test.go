package workloads_test

import (
	"context"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/webhooks"
	"code.cloudfoundry.org/korifi/controllers/webhooks/fake"
	"code.cloudfoundry.org/korifi/controllers/webhooks/workloads"
	"code.cloudfoundry.org/korifi/controllers/webhooks/workloads/integration/helpers"
	"code.cloudfoundry.org/korifi/tests/matchers"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/runtime"
)

var _ = Describe("CFSpaceValidation", func() {
	var (
		ctx                context.Context
		validatingWebhook  *workloads.CFSpaceValidator
		namespace          string
		cfSpace            *korifiv1alpha1.CFSpace
		duplicateValidator *fake.NameValidator
		placementValidator *fake.NamespaceValidator
		retErr             error
	)

	BeforeEach(func() {
		ctx = context.Background()
		namespace = "my-namespace"

		scheme := runtime.NewScheme()
		err := korifiv1alpha1.AddToScheme(scheme)
		Expect(err).NotTo(HaveOccurred())

		cfSpace = &korifiv1alpha1.CFSpace{}

		duplicateValidator = new(fake.NameValidator)
		placementValidator = new(fake.NamespaceValidator)
		validatingWebhook = workloads.NewCFSpaceValidator(duplicateValidator, placementValidator)
	})

	Describe("ValidateCreate", func() {
		BeforeEach(func() {
			cfSpace = helpers.MakeCFSpace(namespace, "my-space")
		})

		JustBeforeEach(func() {
			retErr = validatingWebhook.ValidateCreate(ctx, cfSpace)
		})

		It("allows the request", func() {
			Expect(retErr).NotTo(HaveOccurred())
		})

		It("validates the space name", func() {
			Expect(duplicateValidator.ValidateCreateCallCount()).To(Equal(1))
			_, _, actualNamespace, name, _ := duplicateValidator.ValidateCreateArgsForCall(0)
			Expect(actualNamespace).To(Equal(namespace))
			Expect(name).To(Equal("my-space"))
		})

		When("the space name already exists in the namespace", func() {
			BeforeEach(func() {
				duplicateValidator.ValidateCreateReturns(&webhooks.ValidationError{
					Type:    webhooks.DuplicateNameErrorType,
					Message: "Space '" + cfSpace.Spec.DisplayName + "' already exists. Name must be unique per organization.",
				})
			})

			It("denies the request", func() {
				Expect(retErr).To(matchers.RepresentJSONifiedValidationError(webhooks.ValidationError{
					Type:    webhooks.DuplicateNameErrorType,
					Message: "Space '" + cfSpace.Spec.DisplayName + "' already exists. Name must be unique per organization.",
				}))
			})
		})

		When("the duplicate validator throws a generic error", func() {
			BeforeEach(func() {
				cfSpace = helpers.MakeCFSpace(namespace, "my-space")
				duplicateValidator.ValidateCreateReturns(&webhooks.ValidationError{
					Type:    webhooks.UnknownErrorType,
					Message: webhooks.UnknownErrorMessage,
				})
			})

			It("denies the request", func() {
				Expect(retErr).To(matchers.RepresentJSONifiedValidationError(webhooks.ValidationError{
					Type:    webhooks.UnknownErrorType,
					Message: webhooks.UnknownErrorMessage,
				}))
			})
		})

		When("the placement validator throws an error", func() {
			BeforeEach(func() {
				placementValidator.ValidateSpaceCreateReturns(&webhooks.ValidationError{
					Type:    webhooks.SpacePlacementErrorType,
					Message: "some error",
				})
			})

			It("denies the request", func() {
				Expect(retErr).To(matchers.RepresentJSONifiedValidationError(webhooks.ValidationError{
					Type:    webhooks.SpacePlacementErrorType,
					Message: "some error",
				}))
			})
		})
	})

	Describe("ValidateUpdate", func() {
		var updatedCFSpace *korifiv1alpha1.CFSpace

		BeforeEach(func() {
			cfSpace = helpers.MakeCFSpace(namespace, "my-space")
			updatedCFSpace = helpers.MakeCFSpace(namespace, "another-space")
		})

		JustBeforeEach(func() {
			retErr = validatingWebhook.ValidateUpdate(ctx, cfSpace, updatedCFSpace)
		})

		It("allows the request", func() {
			Expect(retErr).NotTo(HaveOccurred())
		})

		It("invokes the validator correctly", func() {
			Expect(duplicateValidator.ValidateUpdateCallCount()).To(Equal(1))
			actualContext, _, actualNamespace, oldName, newName, _ := duplicateValidator.ValidateUpdateArgsForCall(0)
			Expect(actualContext).To(Equal(ctx))
			Expect(actualNamespace).To(Equal(cfSpace.Namespace))
			Expect(oldName).To(Equal(cfSpace.Spec.DisplayName))
			Expect(newName).To(Equal(updatedCFSpace.Spec.DisplayName))
		})

		When("the new space name already exists in the namespace", func() {
			BeforeEach(func() {
				duplicateValidator.ValidateUpdateReturns(&webhooks.ValidationError{
					Type:    webhooks.DuplicateNameErrorType,
					Message: "Space '" + updatedCFSpace.Spec.DisplayName + "' already exists. Name must be unique per organization.",
				})
			})

			It("denies the request", func() {
				Expect(retErr).To(matchers.RepresentJSONifiedValidationError(webhooks.ValidationError{
					Type:    webhooks.DuplicateNameErrorType,
					Message: "Space '" + updatedCFSpace.Spec.DisplayName + "' already exists. Name must be unique per organization.",
				}))
			})
		})

		When("validate fails for another reason", func() {
			BeforeEach(func() {
				duplicateValidator.ValidateUpdateReturns(&webhooks.ValidationError{
					Type:    webhooks.UnknownErrorType,
					Message: webhooks.UnknownErrorMessage,
				})
			})

			It("denies the request", func() {
				Expect(retErr).To(matchers.RepresentJSONifiedValidationError(webhooks.ValidationError{
					Type:    webhooks.UnknownErrorType,
					Message: webhooks.UnknownErrorMessage,
				}))
			})
		})
	})

	Describe("ValidateDelete", func() {
		BeforeEach(func() {
			cfSpace = helpers.MakeCFSpace(namespace, "my-space")
		})

		JustBeforeEach(func() {
			retErr = validatingWebhook.ValidateDelete(ctx, cfSpace)
		})

		It("allows the request", func() {
			Expect(retErr).NotTo(HaveOccurred())
		})

		It("removes the name from the registry", func() {
			Expect(duplicateValidator.ValidateDeleteCallCount()).To(Equal(1))
			_, _, requestNamespace, name := duplicateValidator.ValidateDeleteArgsForCall(0)
			Expect(requestNamespace).To(Equal(namespace))
			Expect(name).To(Equal("my-space"))
		})

		When("delete validation fails", func() {
			BeforeEach(func() {
				duplicateValidator.ValidateDeleteReturns(&webhooks.ValidationError{
					Type:    webhooks.UnknownErrorType,
					Message: webhooks.UnknownErrorMessage,
				})
			})

			It("disallows the request", func() {
				Expect(retErr).To(matchers.RepresentJSONifiedValidationError(webhooks.ValidationError{
					Type:    webhooks.UnknownErrorType,
					Message: webhooks.UnknownErrorMessage,
				}))
			})
		})
	})
})
