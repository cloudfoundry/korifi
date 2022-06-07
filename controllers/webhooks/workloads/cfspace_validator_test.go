package workloads_test

import (
	"context"
	"errors"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/webhooks"
	"code.cloudfoundry.org/korifi/controllers/webhooks/workloads"
	"code.cloudfoundry.org/korifi/controllers/webhooks/workloads/fake"
	"code.cloudfoundry.org/korifi/controllers/webhooks/workloads/integration/helpers"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/runtime"
)

var _ = Describe("CFSpaceValidation", func() {
	var (
		ctx                     context.Context
		validatingWebhook       *workloads.CFSpaceValidator
		namespace               string
		cfSpace                 *korifiv1alpha1.CFSpace
		orgDuplicateValidator   *fake.NameValidator
		spaceDuplicateValidator *fake.NameValidator
		spacePlacementValidator *fake.PlacementValidator
		retErr                  error
	)

	BeforeEach(func() {
		ctx = context.Background()
		namespace = "my-namespace"
		orgDuplicateValidator = new(fake.NameValidator)
		spaceDuplicateValidator = new(fake.NameValidator)
		spacePlacementValidator = new(fake.PlacementValidator)

		validatingWebhook = workloads.NewCFSpaceValidator(spaceDuplicateValidator, spacePlacementValidator)

		scheme := runtime.NewScheme()
		err := korifiv1alpha1.AddToScheme(scheme)
		Expect(err).NotTo(HaveOccurred())

		cfSpace = &korifiv1alpha1.CFSpace{}
	})

	Describe("ValidateCreate", func() {
		BeforeEach(func() {
			cfSpace = helpers.MakeCFSpace(namespace, "my-space")
		})

		JustBeforeEach(func() {
			retErr = validatingWebhook.ValidateCreate(ctx, cfSpace)
		})

		It("validates the space name", func() {
			Expect(spaceDuplicateValidator.ValidateCreateCallCount()).To(Equal(1))
			Expect(orgDuplicateValidator.ValidateCreateCallCount()).To(Equal(0))
			_, _, actualNamespace, name := spaceDuplicateValidator.ValidateCreateArgsForCall(0)
			Expect(actualNamespace).To(Equal(namespace))
			Expect(name).To(Equal("my-space"))
		})

		When("the space name is unique in the namespace", func() {
			It("allows the request", func() {
				Expect(retErr).NotTo(HaveOccurred())
			})
		})

		When("the space name already exists in the namespace", func() {
			BeforeEach(func() {
				spaceDuplicateValidator.ValidateCreateReturns(webhooks.ErrorDuplicateName)
			})

			It("denies the request", func() {
				Expect(retErr).To(MatchError(MatchJSON(webhooks.ValidationError{
					Type:    workloads.DuplicateSpaceNameErrorType,
					Message: "Space '" + cfSpace.Spec.DisplayName + "' already exists. Name must be unique per organization.",
				}.Marshal())))
			})
		})

		When("duplicate validator throws a generic error", func() {
			BeforeEach(func() {
				cfSpace = helpers.MakeCFSpace(namespace, "my-space")
				spaceDuplicateValidator.ValidateCreateReturns(errors.New("another error"))
			})

			It("denies the request", func() {
				Expect(retErr).To(MatchError(MatchJSON(webhooks.AdmissionUnknownErrorReason())))
			})
		})

		When("placement validator passes", func() {
			BeforeEach(func() {
				spacePlacementValidator.ValidateSpaceCreateReturns(nil)
			})

			It("allows the request", func() {
				Expect(retErr).NotTo(HaveOccurred())
			})
		})

		When("placement validator throws an error", func() {
			BeforeEach(func() {
				spacePlacementValidator.ValidateSpaceCreateReturns(errors.New("some error"))
			})

			It("denies the request", func() {
				Expect(retErr).To(MatchError(MatchJSON(webhooks.ValidationError{
					Type:    workloads.SpacePlacementErrorType,
					Message: "some error",
				}.Marshal())))
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

		When("the space name hasn't changed", func() {
			BeforeEach(func() {
				updatedCFSpace.Labels["something"] = "else"
				updatedCFSpace.Spec.DisplayName = "my-space"
			})

			It("succeeds", func() {
				Expect(retErr).NotTo(HaveOccurred())
			})
		})

		When("the new space name is unique in the namespace", func() {
			It("allows the request", func() {
				Expect(retErr).NotTo(HaveOccurred())
			})
		})

		When("the new space name already exists in the namespace", func() {
			BeforeEach(func() {
				spaceDuplicateValidator.ValidateUpdateReturns(webhooks.ErrorDuplicateName)
			})

			It("denies the request", func() {
				Expect(retErr).To(MatchError(MatchJSON(webhooks.ValidationError{
					Type:    workloads.DuplicateSpaceNameErrorType,
					Message: "Space '" + updatedCFSpace.Spec.DisplayName + "' already exists. Name must be unique per organization.",
				}.Marshal())))
			})
		})

		When("validate fails for another reason", func() {
			BeforeEach(func() {
				spaceDuplicateValidator.ValidateUpdateReturns(errors.New("boom!"))
			})

			It("denies the request", func() {
				Expect(retErr).To(MatchError(MatchJSON(webhooks.AdmissionUnknownErrorReason())))
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
			Expect(spaceDuplicateValidator.ValidateDeleteCallCount()).To(Equal(1))
			_, _, requestNamespace, name := spaceDuplicateValidator.ValidateDeleteArgsForCall(0)
			Expect(requestNamespace).To(Equal(namespace))
			Expect(name).To(Equal("my-space"))
		})

		When("delete validation fails", func() {
			BeforeEach(func() {
				spaceDuplicateValidator.ValidateDeleteReturns(errors.New("boom!"))
			})

			It("disallows the request", func() {
				Expect(retErr).To(MatchError(webhooks.AdmissionUnknownErrorReason()))
			})
		})
	})
})
