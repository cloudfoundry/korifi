package workloads_test

import (
	"context"
	"encoding/json"
	"errors"

	"code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/webhooks"
	"code.cloudfoundry.org/korifi/controllers/webhooks/workloads"
	"code.cloudfoundry.org/korifi/controllers/webhooks/workloads/fake"
	"code.cloudfoundry.org/korifi/controllers/webhooks/workloads/integration/helpers"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	admissionv1 "k8s.io/api/admission/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

var _ = Describe("CFSpaceValidation", func() {
	var (
		ctx                     context.Context
		validatingWebhook       *workloads.CFSpaceValidation
		namespace               string
		cfSpace                 *v1alpha1.CFSpace
		orgDuplicateValidator   *fake.NameValidator
		spaceDuplicateValidator *fake.NameValidator
		spacePlacementValidator *fake.PlacementValidator
		request                 admission.Request
		response                admission.Response
	)

	BeforeEach(func() {
		ctx = context.Background()
		namespace = "my-namespace"
		orgDuplicateValidator = new(fake.NameValidator)
		spaceDuplicateValidator = new(fake.NameValidator)
		spacePlacementValidator = new(fake.PlacementValidator)

		validatingWebhook = workloads.NewCFSpaceValidation(spaceDuplicateValidator, spacePlacementValidator)

		scheme := runtime.NewScheme()
		err := v1alpha1.AddToScheme(scheme)
		Expect(err).NotTo(HaveOccurred())

		decoder, err := admission.NewDecoder(scheme)
		Expect(err).NotTo(HaveOccurred())
		Expect(validatingWebhook.InjectDecoder(decoder)).To(Succeed())

		cfSpace = &v1alpha1.CFSpace{}
	})

	Describe("Create", func() {
		BeforeEach(func() {
			cfSpace = helpers.MakeCFSpace(namespace, "my-space")
		})

		JustBeforeEach(func() {
			cfSpaceJSON, err := json.Marshal(cfSpace)
			Expect(err).NotTo(HaveOccurred())

			request = admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Name:      cfSpace.Name,
					Namespace: namespace,
					Operation: admissionv1.Create,
					Object: runtime.RawExtension{
						Raw: cfSpaceJSON,
					},
				},
			}
			response = validatingWebhook.Handle(ctx, request)
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
				Expect(response.Allowed).To(BeTrue())
			})
		})

		When("the space name already exists in the namespace", func() {
			BeforeEach(func() {
				spaceDuplicateValidator.ValidateCreateReturns(webhooks.ErrorDuplicateName)
			})

			It("denies the request", func() {
				Expect(response.Allowed).To(BeFalse())
			})
		})

		When("duplicate validator throws a generic error", func() {
			BeforeEach(func() {
				cfSpace = helpers.MakeCFSpace(namespace, "my-space")
				spaceDuplicateValidator.ValidateCreateReturns(errors.New("another error"))
			})

			It("denies the request", func() {
				Expect(response.Allowed).To(BeFalse())
			})
		})

		When("placement validator passes", func() {
			BeforeEach(func() {
				spacePlacementValidator.ValidateSpaceCreateReturns(nil)
			})

			It("allows the request", func() {
				Expect(response.Allowed).To(BeTrue())
			})
		})

		When("placement validator throws an error", func() {
			BeforeEach(func() {
				spacePlacementValidator.ValidateSpaceCreateReturns(errors.New("some error"))
			})

			It("denies the request", func() {
				Expect(response.Allowed).To(BeFalse())
			})
		})
	})

	Describe("Update", func() {
		var newCFSpace *v1alpha1.CFSpace

		BeforeEach(func() {
			cfSpace = helpers.MakeCFSpace(namespace, "my-space")
			newCFSpace = helpers.MakeCFSpace(namespace, "another-space")
		})

		JustBeforeEach(func() {
			cfSpaceJSON, err := json.Marshal(cfSpace)
			Expect(err).NotTo(HaveOccurred())

			newCFSpaceJSON, err := json.Marshal(newCFSpace)
			Expect(err).NotTo(HaveOccurred())

			request = admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Name:      cfSpace.Name,
					Namespace: namespace,
					Operation: admissionv1.Update,
					Object: runtime.RawExtension{
						Raw: newCFSpaceJSON,
					},
					OldObject: runtime.RawExtension{
						Raw: cfSpaceJSON,
					},
				},
			}
			response = validatingWebhook.Handle(ctx, request)
		})

		When("the space name hasn't changed", func() {
			BeforeEach(func() {
				newCFSpace.Labels["something"] = "else"
				newCFSpace.Spec.DisplayName = "my-space"
			})

			It("succeeds", func() {
				Expect(response.Allowed).To(BeTrue())
			})
		})

		When("the new space name is unique in the namespace", func() {
			It("allows the request", func() {
				Expect(response.Allowed).To(BeTrue())
			})
		})

		When("the new space name already exists in the namespace", func() {
			BeforeEach(func() {
				spaceDuplicateValidator.ValidateUpdateReturns(webhooks.ErrorDuplicateName)
			})

			It("denies the request", func() {
				Expect(response.Allowed).To(BeFalse())
			})
		})

		When("validate fails for another reason", func() {
			BeforeEach(func() {
				spaceDuplicateValidator.ValidateUpdateReturns(errors.New("boom!"))
			})

			It("denies the request", func() {
				Expect(response.Allowed).To(BeFalse())
			})
		})

		Context("failures", func() {
			When("decoding fails", func() {
				It("denies the request", func() {
					request.Object.Raw = []byte(`"[1,`)
					response = validatingWebhook.Handle(ctx, request)
					Expect(response.Allowed).To(BeFalse())
				})

				It("does not attempt to lock any names", func() {
					// ignore the calls from the JustBeforeEach()
					spaceValidateUpdateCount := spaceDuplicateValidator.ValidateUpdateCallCount()

					request.Object.Raw = []byte(`"[1,`)
					response = validatingWebhook.Handle(ctx, request)
					Expect(spaceDuplicateValidator.ValidateUpdateCallCount()).To(Equal(spaceValidateUpdateCount))
				})
			})
		})
	})

	Describe("Deletion", func() {
		BeforeEach(func() {
			cfSpace = helpers.MakeCFSpace(namespace, "my-space")
		})

		JustBeforeEach(func() {
			cfSpaceJSON, err := json.Marshal(cfSpace)
			Expect(err).NotTo(HaveOccurred())
			request = admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Name:      cfSpace.Name,
					Namespace: namespace,
					Operation: admissionv1.Delete,
					OldObject: runtime.RawExtension{
						Raw: cfSpaceJSON,
					},
				},
			}
			response = validatingWebhook.Handle(ctx, request)
		})

		It("removes the name from the registry", func() {
			Expect(spaceDuplicateValidator.ValidateDeleteCallCount()).To(Equal(1))
			_, _, requestNamespace, name := spaceDuplicateValidator.ValidateDeleteArgsForCall(0)
			Expect(requestNamespace).To(Equal(namespace))
			Expect(name).To(Equal("my-space"))
		})
	})

	Describe("Request validation", func() {
		BeforeEach(func() {
			request = admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Name:      cfSpace.Name,
					Namespace: namespace,
					Operation: admissionv1.Create,
					Object: runtime.RawExtension{
						Raw: []byte(`"[1,`),
					},
				},
			}
			response = validatingWebhook.Handle(ctx, request)
		})
		It("denies the request", func() {
			Expect(response.Allowed).To(BeFalse())
		})

		It("does not attempt to register any names", func() {
			Expect(spaceDuplicateValidator.ValidateCreateCallCount()).To(Equal(0))
		})
	})
})
