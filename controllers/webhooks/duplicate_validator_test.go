package webhooks_test

import (
	"context"
	"errors"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/webhooks"
	"code.cloudfoundry.org/korifi/controllers/webhooks/fake"
	"code.cloudfoundry.org/korifi/tests/matchers"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

var _ = Describe("DuplicateValidator", func() {
	var (
		ctx                context.Context
		nameRegistry       *fake.NameRegistry
		logger             logr.Logger
		duplicateValidator *webhooks.DuplicateValidator
		validationErr      error
		uniqueClientObj    *fake.UniqueClientObject
	)

	BeforeEach(func() {
		ctx = context.Background()

		scheme := runtime.NewScheme()
		Expect(korifiv1alpha1.AddToScheme(scheme)).To(Succeed())

		nameRegistry = new(fake.NameRegistry)
		duplicateValidator = webhooks.NewDuplicateValidator(nameRegistry)
		logger = logf.Log

		uniqueClientObj = new(fake.UniqueClientObject)
		uniqueClientObj.GetNameReturns("test-resource-name")
		uniqueClientObj.GetNamespaceReturns("test-resource-namespace")
		uniqueClientObj.UniqueNameReturns("unique-name")
		uniqueClientObj.UniqueValidationErrorMessageReturns("uniqueness-error")
	})

	Describe("ValidateCreate", func() {
		JustBeforeEach(func() {
			validationErr = duplicateValidator.ValidateCreate(ctx, logger, "uniqueness-namespace", uniqueClientObj)
		})

		It("uses the name registry to register the name", func() {
			Expect(validationErr).NotTo(HaveOccurred())

			Expect(nameRegistry.RegisterNameCallCount()).To(Equal(1))
			actualContext, namespace, name, ownerNamespace, ownerName := nameRegistry.RegisterNameArgsForCall(0)

			Expect(actualContext).To(Equal(ctx))
			Expect(namespace).To(Equal("uniqueness-namespace"))
			Expect(name).To(Equal("unique-name"))
			Expect(ownerNamespace).To(Equal("test-resource-namespace"))
			Expect(ownerName).To(Equal("test-resource-name"))
		})

		When("the name is already registered", func() {
			BeforeEach(func() {
				nameRegistry.RegisterNameReturns(k8serrors.NewAlreadyExists(schema.GroupResource{}, "jim"))
			})

			It("fails", func() {
				Expect(validationErr).To(matchers.BeValidationError(
					webhooks.DuplicateNameErrorType,
					Equal("uniqueness-error"),
				))
			})
		})

		When("registering the name fails", func() {
			BeforeEach(func() {
				nameRegistry.RegisterNameReturns(errors.New("register-err"))
			})

			It("returns the error", func() {
				Expect(validationErr).To(matchers.BeValidationError(
					webhooks.UnknownErrorType,
					Equal(webhooks.UnknownErrorMessage),
				))
			})
		})
	})

	Describe("ValidateUpdate", func() {
		var newUniqueClientObj *fake.UniqueClientObject
		BeforeEach(func() {
			newUniqueClientObj = new(fake.UniqueClientObject)
			newUniqueClientObj = new(fake.UniqueClientObject)
			newUniqueClientObj.GetNameReturns("test-resource-name")
			newUniqueClientObj.GetNamespaceReturns("test-resource-namespace")
			newUniqueClientObj.UniqueNameReturns("new-unique-name")
			newUniqueClientObj.UniqueValidationErrorMessageReturns("new-uniqueness-error")
		})

		JustBeforeEach(func() {
			validationErr = duplicateValidator.ValidateUpdate(ctx, logger, "uniqueness-namespace", uniqueClientObj, newUniqueClientObj)
		})

		It("locks the old name, registers the new name, deregisters the old name", func() {
			Expect(validationErr).NotTo(HaveOccurred())

			Expect(nameRegistry.TryLockNameCallCount()).To(Equal(1))
			_, namespace, name := nameRegistry.TryLockNameArgsForCall(0)
			Expect(namespace).To(Equal("uniqueness-namespace"))
			Expect(name).To(Equal("unique-name"))

			Expect(nameRegistry.RegisterNameCallCount()).To(Equal(1))
			_, namespace, name, ownerNamespace, ownerName := nameRegistry.RegisterNameArgsForCall(0)
			Expect(namespace).To(Equal("uniqueness-namespace"))
			Expect(name).To(Equal("new-unique-name"))
			Expect(ownerNamespace).To(Equal("test-resource-namespace"))
			Expect(ownerName).To(Equal("test-resource-name"))

			Expect(nameRegistry.DeregisterNameCallCount()).To(Equal(1))
			_, namespace, name = nameRegistry.DeregisterNameArgsForCall(0)
			Expect(namespace).To(Equal("uniqueness-namespace"))
			Expect(name).To(Equal("unique-name"))
		})

		When("the name isn't changed", func() {
			BeforeEach(func() {
				newUniqueClientObj.UniqueNameReturns("unique-name")
			})

			It("is allowed without using the name registry", func() {
				Expect(validationErr).NotTo(HaveOccurred())
				Expect(nameRegistry.TryLockNameCallCount()).To(Equal(0))
				Expect(nameRegistry.RegisterNameCallCount()).To(Equal(0))
				Expect(nameRegistry.DeregisterNameCallCount()).To(Equal(0))
			})
		})

		When("taking the lock on the old name fails", func() {
			BeforeEach(func() {
				nameRegistry.TryLockNameReturns(errors.New("nope"))
			})

			It("fails", func() {
				Expect(validationErr).To(matchers.BeValidationError(
					webhooks.UnknownErrorType,
					Equal(webhooks.UnknownErrorMessage),
				))
			})

			It("does not register the new name", func() {
				Expect(nameRegistry.RegisterNameCallCount()).To(Equal(0))
			})
		})

		When("the new name is already used", func() {
			BeforeEach(func() {
				nameRegistry.RegisterNameReturns(k8serrors.NewAlreadyExists(schema.GroupResource{}, "foo"))
			})

			It("fails", func() {
				Expect(validationErr).To(matchers.BeValidationError(
					webhooks.DuplicateNameErrorType,
					Equal("new-uniqueness-error"),
				))
			})
		})

		When("registering the new name fails", func() {
			BeforeEach(func() {
				nameRegistry.RegisterNameReturns(errors.New("register-err"))
			})

			It("denies the request", func() {
				Expect(validationErr).To(matchers.BeValidationError(
					webhooks.UnknownErrorType,
					Equal(webhooks.UnknownErrorMessage),
				))
			})

			It("releases the lock on the old name", func() {
				Expect(nameRegistry.UnlockNameCallCount()).To(Equal(1))
				_, namespace, name := nameRegistry.UnlockNameArgsForCall(0)
				Expect(namespace).To(Equal("uniqueness-namespace"))
				Expect(name).To(Equal("unique-name"))
			})

			When("releasing the old name lock fails", func() {
				BeforeEach(func() {
					nameRegistry.UnlockNameReturns(errors.New("unlock-err"))
				})

				It("fails with the register error", func() {
					Expect(validationErr).To(matchers.BeValidationError(
						webhooks.UnknownErrorType,
						Equal(webhooks.UnknownErrorMessage),
					))
				})
			})
		})

		When("releasing the old name fails", func() {
			BeforeEach(func() {
				nameRegistry.DeregisterNameReturns(errors.New("deregister-err"))
			})

			It("continues anyway and allows the request", func() {
				Expect(validationErr).NotTo(HaveOccurred())
			})
		})
	})

	Describe("ValidateDelete", func() {
		JustBeforeEach(func() {
			validationErr = duplicateValidator.ValidateDelete(ctx, logger, "uniqueness-namespace", uniqueClientObj)
		})

		It("succeeds", func() {
			Expect(validationErr).NotTo(HaveOccurred())
		})

		It("deregisters the old name", func() {
			Expect(nameRegistry.DeregisterNameCallCount()).To(Equal(1))
			_, namespace, name := nameRegistry.DeregisterNameArgsForCall(0)
			Expect(namespace).To(Equal("uniqueness-namespace"))
			Expect(name).To(Equal("unique-name"))
		})

		When("the old name doesn't exist", func() {
			BeforeEach(func() {
				nameRegistry.DeregisterNameReturns(k8serrors.NewNotFound(schema.GroupResource{}, "foo"))
			})

			It("succeeds", func() {
				Expect(validationErr).NotTo(HaveOccurred())
			})
		})

		When("deregistering fails", func() {
			BeforeEach(func() {
				nameRegistry.DeregisterNameReturns(errors.New("deregister-err"))
			})

			It("fails", func() {
				Expect(validationErr).To(matchers.BeValidationError(
					webhooks.UnknownErrorType,
					Equal(webhooks.UnknownErrorMessage),
				))
			})
		})
	})
})
