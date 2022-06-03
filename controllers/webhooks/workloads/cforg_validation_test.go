package workloads_test

import (
	"context"
	"errors"
	"fmt"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/webhooks"
	"code.cloudfoundry.org/korifi/controllers/webhooks/workloads"
	workloadsfake "code.cloudfoundry.org/korifi/controllers/webhooks/workloads/fake"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

var _ = Describe("CFOrgValidatingWebhook", func() {
	const (
		testOrgGUID   = "test-org-guid"
		testOrgName   = "test-org"
		rootNamespace = "cf"
	)

	var (
		ctx                context.Context
		duplicateValidator *workloadsfake.NameValidator
		placementValidator *workloadsfake.PlacementValidator
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

		duplicateValidator = new(workloadsfake.NameValidator)
		placementValidator = new(workloadsfake.PlacementValidator)
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
			actualContext, _, namespace, name := duplicateValidator.ValidateCreateArgsForCall(0)
			Expect(actualContext).To(Equal(ctx))
			Expect(namespace).To(Equal(rootNamespace))
			Expect(name).To(Equal(testOrgName))
		})

		When("the org name is a duplicate", func() {
			BeforeEach(func() {
				duplicateValidator.ValidateCreateReturns(webhooks.ErrorDuplicateName)
			})

			It("denies the request", func() {
				Expect(retErr).To(MatchError(webhooks.ValidationError{
					Type:    workloads.DuplicateOrgNameErrorType,
					Message: "Organization '" + cfOrg.Spec.DisplayName + "' already exists.",
				}.Marshal()))
			})
		})

		When("validating the org name fails", func() {
			BeforeEach(func() {
				duplicateValidator.ValidateCreateReturns(errors.New("boom"))
			})

			It("denies the request", func() {
				Expect(retErr).To(MatchError(webhooks.AdmissionUnknownErrorReason()))
			})
		})

		When("the org placement validator fails", func() {
			BeforeEach(func() {
				placementValidator.ValidateOrgCreateReturns(fmt.Errorf(webhooks.OrgPlacementErrorMessage, cfOrg.Spec.DisplayName))
			})

			It("denies the request", func() {
				Expect(retErr).To(MatchError(webhooks.ValidationError{
					Type:    workloads.OrgPlacementErrorType,
					Message: "Organization '" + cfOrg.Spec.DisplayName + "' must be placed in the root 'cf' namespace",
				}.Marshal()))
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
			actualContext, _, namespace, oldName, newName := duplicateValidator.ValidateUpdateArgsForCall(0)
			Expect(actualContext).To(Equal(ctx))
			Expect(namespace).To(Equal(cfOrg.Namespace))
			Expect(oldName).To(Equal(cfOrg.Spec.DisplayName))
			Expect(newName).To(Equal(updatedCFOrg.Spec.DisplayName))
		})

		When("the new org name is a duplicate", func() {
			BeforeEach(func() {
				duplicateValidator.ValidateUpdateReturns(webhooks.ErrorDuplicateName)
			})

			It("denies the request", func() {
				Expect(retErr).To(MatchError(webhooks.ValidationError{
					Type:    workloads.DuplicateOrgNameErrorType,
					Message: "Organization '" + updatedCFOrg.Spec.DisplayName + "' already exists.",
				}.Marshal()))
			})
		})

		When("the update validation fails for another reason", func() {
			BeforeEach(func() {
				duplicateValidator.ValidateUpdateReturns(errors.New("boom!"))
			})

			It("denies the request", func() {
				Expect(retErr).To(MatchError(webhooks.AdmissionUnknownErrorReason()))
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
			actualContext, _, namespace, name := duplicateValidator.ValidateDeleteArgsForCall(0)
			Expect(actualContext).To(Equal(ctx))
			Expect(namespace).To(Equal(cfOrg.Namespace))
			Expect(name).To(Equal(cfOrg.Spec.DisplayName))
		})

		When("delete validation fails", func() {
			BeforeEach(func() {
				duplicateValidator.ValidateDeleteReturns(errors.New("boom!"))
			})

			It("disallows the request", func() {
				Expect(retErr).To(MatchError(webhooks.AdmissionUnknownErrorReason()))
			})
		})
	})
})
