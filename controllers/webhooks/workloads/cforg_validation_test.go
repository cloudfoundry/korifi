package workloads_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	workloadsv1alpha1 "code.cloudfoundry.org/korifi/controllers/apis/workloads/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/webhooks"
	"code.cloudfoundry.org/korifi/controllers/webhooks/workloads"
	workloadsfake "code.cloudfoundry.org/korifi/controllers/webhooks/workloads/fake"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	admissionv1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
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
		realDecoder        *admission.Decoder
		org                *workloadsv1alpha1.CFOrg
		request            admission.Request
		validatingWebhook  *workloads.CFOrgValidation
		response           admission.Response
		cfOrgJSON          []byte
	)

	BeforeEach(func() {
		ctx = context.Background()

		scheme := runtime.NewScheme()
		err := workloadsv1alpha1.AddToScheme(scheme)
		Expect(err).NotTo(HaveOccurred())

		realDecoder, err = admission.NewDecoder(scheme)
		Expect(err).NotTo(HaveOccurred())

		org = &workloadsv1alpha1.CFOrg{
			ObjectMeta: metav1.ObjectMeta{
				Name:      testOrgGUID,
				Namespace: rootNamespace,
			},
			Spec: workloadsv1alpha1.CFOrgSpec{
				DisplayName: testOrgName,
			},
		}

		cfOrgJSON, err = json.Marshal(org)
		Expect(err).NotTo(HaveOccurred())

		duplicateValidator = new(workloadsfake.NameValidator)
		placementValidator = new(workloadsfake.PlacementValidator)
		validatingWebhook = workloads.NewCFOrgValidation(duplicateValidator, placementValidator)

		Expect(validatingWebhook.InjectDecoder(realDecoder)).To(Succeed())
	})

	Describe("Create", func() {
		JustBeforeEach(func() {
			response = validatingWebhook.Handle(ctx, request)
		})

		BeforeEach(func() {
			request = admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Name:      testOrgGUID,
					Namespace: rootNamespace,
					Operation: admissionv1.Create,
					Object: runtime.RawExtension{
						Raw: cfOrgJSON,
					},
				},
			}
		})

		It("allows the request", func() {
			Expect(response.Allowed).To(BeTrue())
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
				Expect(response.Allowed).To(BeFalse())
				Expect(string(response.Result.Reason)).To(Equal(webhooks.ValidationError{Type: workloads.DuplicateOrgErrorType, Message: `Organization '` + org.Spec.DisplayName + `' already exists.`}.Marshal()))
			})
		})

		When("the org placement validator passes", func() {
			BeforeEach(func() {
				placementValidator.ValidateOrgCreateReturns(nil)
			})

			It("allows the request", func() {
				Expect(response.Allowed).To(BeTrue())
			})
		})

		When("the org placement validator fails", func() {
			BeforeEach(func() {
				placementValidator.ValidateOrgCreateReturns(fmt.Errorf(webhooks.OrgPlacementErrorMessage, org.Spec.DisplayName))
			})

			It("denies the request", func() {
				Expect(response.Allowed).To(BeFalse())
				Expect(string(response.Result.Reason)).To(Equal(webhooks.ValidationError{Type: workloads.OrgPlacementErrorType, Message: `Organization '` + org.Spec.DisplayName + `' must be placed in the root 'cf' namespace`}.Marshal()))
			})
		})

		When("there is an issue decoding the request", func() {
			BeforeEach(func() {
				cfOrgJSON, err := json.Marshal(org)
				Expect(err).NotTo(HaveOccurred())
				badCFOrgJSON := []byte("}" + string(cfOrgJSON))

				request = admission.Request{
					AdmissionRequest: admissionv1.AdmissionRequest{
						Name:      testOrgGUID,
						Namespace: rootNamespace,
						Operation: admissionv1.Create,
						Object: runtime.RawExtension{
							Raw: badCFOrgJSON,
						},
					},
				}
			})

			It("denies the request", func() {
				Expect(response.Allowed).To(BeFalse())
			})

			It("does not attempt to register a name", func() {
				Expect(duplicateValidator.ValidateCreateCallCount()).To(Equal(0))
			})
		})

		When("validating the org name fails", func() {
			BeforeEach(func() {
				duplicateValidator.ValidateCreateReturns(errors.New("boom"))
			})

			It("denies the request", func() {
				Expect(response.Allowed).To(BeFalse())
				Expect(string(response.Result.Reason)).To(Equal(webhooks.AdmissionUnknownErrorReason()))
			})
		})
	})

	Describe("Update", func() {
		var updatedOrg *workloadsv1alpha1.CFOrg

		BeforeEach(func() {
			updatedOrg = &workloadsv1alpha1.CFOrg{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testOrgGUID,
					Namespace: rootNamespace,
				},
				Spec: workloadsv1alpha1.CFOrgSpec{
					DisplayName: "the-new-name",
				},
			}
		})

		JustBeforeEach(func() {
			orgJSON, err := json.Marshal(org)
			Expect(err).NotTo(HaveOccurred())

			updatedOrgJSON, err := json.Marshal(updatedOrg)
			Expect(err).NotTo(HaveOccurred())

			request = admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Name:      testOrgGUID,
					Namespace: rootNamespace,
					Operation: admissionv1.Update,
					Object: runtime.RawExtension{
						Raw: updatedOrgJSON,
					},
					OldObject: runtime.RawExtension{
						Raw: orgJSON,
					},
				},
			}

			response = validatingWebhook.Handle(ctx, request)
		})

		It("allows the request", func() {
			Expect(response.Allowed).To(BeTrue())
		})

		It("invokes the validator correctly", func() {
			Expect(duplicateValidator.ValidateUpdateCallCount()).To(Equal(1))
			actualContext, _, namespace, oldName, newName := duplicateValidator.ValidateUpdateArgsForCall(0)
			Expect(actualContext).To(Equal(ctx))
			Expect(namespace).To(Equal(org.Namespace))
			Expect(oldName).To(Equal(org.Spec.DisplayName))
			Expect(newName).To(Equal(updatedOrg.Spec.DisplayName))
		})

		When("the new org name is a duplicate", func() {
			BeforeEach(func() {
				duplicateValidator.ValidateUpdateReturns(webhooks.ErrorDuplicateName)
			})

			It("denies the request", func() {
				Expect(response.Allowed).To(BeFalse())
				Expect(string(response.Result.Reason)).To(Equal(webhooks.ValidationError{Type: workloads.DuplicateOrgErrorType, Message: `Organization '` + updatedOrg.Spec.DisplayName + `' already exists.`}.Marshal()))
			})
		})

		When("the update validation fails for another reason", func() {
			BeforeEach(func() {
				duplicateValidator.ValidateUpdateReturns(errors.New("boom!"))
			})

			It("denies the request", func() {
				Expect(response.Allowed).To(BeFalse())
				Expect(string(response.Result.Reason)).To(Equal(webhooks.AdmissionUnknownErrorReason()))
			})
		})
	})

	Describe("Delete", func() {
		JustBeforeEach(func() {
			orgJSON, err := json.Marshal(org)
			Expect(err).NotTo(HaveOccurred())

			request = admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Name:      testOrgGUID,
					Namespace: rootNamespace,
					Operation: admissionv1.Delete,
					OldObject: runtime.RawExtension{
						Raw: orgJSON,
					},
				},
			}

			response = validatingWebhook.Handle(ctx, request)
		})

		It("allows the request", func() {
			Expect(response.Allowed).To(BeTrue())
		})

		It("invokes the validator correctly", func() {
			Expect(duplicateValidator.ValidateDeleteCallCount()).To(Equal(1))
			actualContext, _, namespace, name := duplicateValidator.ValidateDeleteArgsForCall(0)
			Expect(actualContext).To(Equal(ctx))
			Expect(namespace).To(Equal(org.Namespace))
			Expect(name).To(Equal(org.Spec.DisplayName))
		})

		When("delete validation fails", func() {
			BeforeEach(func() {
				duplicateValidator.ValidateDeleteReturns(errors.New("boom!"))
			})

			It("disallows the request", func() {
				Expect(response.Allowed).To(BeFalse())
				Expect(string(response.Result.Reason)).To(Equal(webhooks.AdmissionUnknownErrorReason()))
			})
		})
	})
})
