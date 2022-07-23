package workloads_test

import (
	"context"
	"fmt"
	"time"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/webhooks"
	"code.cloudfoundry.org/korifi/controllers/webhooks/fake"
	"code.cloudfoundry.org/korifi/controllers/webhooks/workloads"
	"code.cloudfoundry.org/korifi/tests/matchers"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

var _ = Describe("CFOrgValidator", func() {
	const (
		testOrgGUID   = "test-org-guid"
		testOrgName   = "test-org"
		rootNamespace = "cf"
	)

	var (
		ctx                context.Context
		duplicateValidator *fake.NameValidator
		placementValidator *fake.NamespaceValidator
		cfOrg              *korifiv1alpha1.CFOrg
		validatingWebhook  *workloads.CFOrgValidator
		retErr             error
	)

	BeforeEach(func() {
		ctx = context.Background()

		scheme := runtime.NewScheme()
		err := korifiv1alpha1.AddToScheme(scheme)
		Expect(err).NotTo(HaveOccurred())

		cfOrg = &korifiv1alpha1.CFOrg{
			ObjectMeta: metav1.ObjectMeta{
				Name:      testOrgGUID,
				Namespace: rootNamespace,
			},
			Spec: korifiv1alpha1.CFOrgSpec{
				DisplayName: testOrgName,
			},
		}

		duplicateValidator = new(fake.NameValidator)
		placementValidator = new(fake.NamespaceValidator)
		validatingWebhook = workloads.NewCFOrgValidator(duplicateValidator, placementValidator)
	})

	Describe("ValidateCreate", func() {
		JustBeforeEach(func() {
			retErr = validatingWebhook.ValidateCreate(ctx, cfOrg)
		})

		It("allows the request", func() {
			Expect(retErr).NotTo(HaveOccurred())
		})

		It("invokes the validator correctly", func() {
			Expect(duplicateValidator.ValidateCreateCallCount()).To(Equal(1))
			actualContext, _, actualNamespace, name, _ := duplicateValidator.ValidateCreateArgsForCall(0)
			Expect(actualContext).To(Equal(ctx))
			Expect(actualNamespace).To(Equal(rootNamespace))
			Expect(name).To(Equal(testOrgName))
		})

		When("the org name is a duplicate", func() {
			BeforeEach(func() {
				duplicateValidator.ValidateCreateReturns(&webhooks.ValidationError{
					Type:    webhooks.DuplicateNameErrorType,
					Message: "Organization '" + cfOrg.Spec.DisplayName + "' already exists.",
				})
			})

			It("denies the request", func() {
				Expect(retErr).To(matchers.RepresentJSONifiedValidationError(webhooks.ValidationError{
					Type:    webhooks.DuplicateNameErrorType,
					Message: "Organization '" + cfOrg.Spec.DisplayName + "' already exists.",
				}))
			})
		})

		When("validating the org name fails", func() {
			BeforeEach(func() {
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

		When("the org placement validator fails", func() {
			BeforeEach(func() {
				placementValidator.ValidateOrgCreateReturns(&webhooks.ValidationError{
					Type:    webhooks.OrgPlacementErrorType,
					Message: fmt.Sprintf(webhooks.OrgPlacementErrorMessage, cfOrg.Spec.DisplayName),
				})
			})

			It("denies the request", func() {
				Expect(retErr).To(matchers.RepresentJSONifiedValidationError(webhooks.ValidationError{
					Type:    webhooks.OrgPlacementErrorType,
					Message: fmt.Sprintf(webhooks.OrgPlacementErrorMessage, cfOrg.Spec.DisplayName),
				}))
			})
		})
	})

	Describe("ValidateUpdate", func() {
		var updatedCFOrg *korifiv1alpha1.CFOrg

		BeforeEach(func() {
			updatedCFOrg = cfOrg.DeepCopy()
			updatedCFOrg.Spec.DisplayName = "the-new-name"
		})

		JustBeforeEach(func() {
			retErr = validatingWebhook.ValidateUpdate(ctx, cfOrg, updatedCFOrg)
		})

		It("allows the request", func() {
			Expect(retErr).NotTo(HaveOccurred())
		})

		It("invokes the validator correctly", func() {
			Expect(duplicateValidator.ValidateUpdateCallCount()).To(Equal(1))
			actualContext, _, actualNamespace, oldName, newName, _ := duplicateValidator.ValidateUpdateArgsForCall(0)
			Expect(actualContext).To(Equal(ctx))
			Expect(actualNamespace).To(Equal(cfOrg.Namespace))
			Expect(oldName).To(Equal(cfOrg.Spec.DisplayName))
			Expect(newName).To(Equal(updatedCFOrg.Spec.DisplayName))
		})

		When("the org is being deleted", func() {
			BeforeEach(func() {
				updatedCFOrg.DeletionTimestamp = &metav1.Time{Time: time.Now()}
			})

			It("does not return an error", func() {
				Expect(retErr).NotTo(HaveOccurred())
			})
		})

		When("the new org name is a duplicate", func() {
			BeforeEach(func() {
				duplicateValidator.ValidateUpdateReturns(&webhooks.ValidationError{
					Type:    webhooks.DuplicateNameErrorType,
					Message: "Organization '" + updatedCFOrg.Spec.DisplayName + "' already exists.",
				})
			})

			It("denies the request", func() {
				Expect(retErr).To(matchers.RepresentJSONifiedValidationError(webhooks.ValidationError{
					Type:    webhooks.DuplicateNameErrorType,
					Message: "Organization '" + updatedCFOrg.Spec.DisplayName + "' already exists.",
				}))
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
				Expect(retErr).To(matchers.RepresentJSONifiedValidationError(webhooks.ValidationError{
					Type:    webhooks.UnknownErrorType,
					Message: webhooks.UnknownErrorMessage,
				}))
			})
		})
	})

	Describe("ValidateDelete", func() {
		JustBeforeEach(func() {
			retErr = validatingWebhook.ValidateDelete(ctx, cfOrg)
		})

		It("allows the request", func() {
			Expect(retErr).NotTo(HaveOccurred())
		})

		It("invokes the validator correctly", func() {
			Expect(duplicateValidator.ValidateDeleteCallCount()).To(Equal(1))
			actualContext, _, actualNamespace, name := duplicateValidator.ValidateDeleteArgsForCall(0)
			Expect(actualContext).To(Equal(ctx))
			Expect(actualNamespace).To(Equal(cfOrg.Namespace))
			Expect(name).To(Equal(cfOrg.Spec.DisplayName))
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
