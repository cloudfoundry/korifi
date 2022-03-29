package services_test

import (
	"context"
	"encoding/json"
	"errors"

	servicesv1alpha1 "code.cloudfoundry.org/cf-k8s-controllers/controllers/apis/services/v1alpha1"
	"code.cloudfoundry.org/cf-k8s-controllers/controllers/webhooks"
	"code.cloudfoundry.org/cf-k8s-controllers/controllers/webhooks/services"
	"code.cloudfoundry.org/cf-k8s-controllers/controllers/webhooks/services/fake"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	admissionv1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

var _ = Describe("CFServiceInstanceValidatingWebhook", func() {
	const (
		defaultNamespace = "default"
	)

	var (
		serviceInstanceGUID   string
		serviceInstanceName   string
		ctx                   context.Context
		duplicateValidator    *fake.NameValidator
		realDecoder           *admission.Decoder
		serviceInstance       *servicesv1alpha1.CFServiceInstance
		request               admission.Request
		validatingWebhook     *services.CFServiceInstanceValidation
		response              admission.Response
		cfServiceInstanceJSON []byte
	)

	BeforeEach(func() {
		ctx = context.Background()

		scheme := runtime.NewScheme()
		err := servicesv1alpha1.AddToScheme(scheme)
		Expect(err).NotTo(HaveOccurred())

		realDecoder, err = admission.NewDecoder(scheme)
		Expect(err).NotTo(HaveOccurred())

		serviceInstanceName = generateGUID("service-instance")
		serviceInstanceGUID = generateGUID("service-instance")
		serviceInstance = &servicesv1alpha1.CFServiceInstance{
			ObjectMeta: metav1.ObjectMeta{
				Name:      serviceInstanceGUID,
				Namespace: defaultNamespace,
			},
			Spec: servicesv1alpha1.CFServiceInstanceSpec{
				Name: serviceInstanceName,
				Type: servicesv1alpha1.UserProvidedType,
			},
		}

		cfServiceInstanceJSON, err = json.Marshal(serviceInstance)
		Expect(err).NotTo(HaveOccurred())

		duplicateValidator = new(fake.NameValidator)
		validatingWebhook = services.NewCFServiceInstanceValidation(duplicateValidator)

		Expect(validatingWebhook.InjectDecoder(realDecoder)).To(Succeed())
	})

	Describe("Create", func() {
		JustBeforeEach(func() {
			response = validatingWebhook.Handle(ctx, request)
		})

		BeforeEach(func() {
			request = admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Name:      serviceInstanceGUID,
					Namespace: defaultNamespace,
					Operation: admissionv1.Create,
					Object: runtime.RawExtension{
						Raw: cfServiceInstanceJSON,
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
			Expect(namespace).To(Equal(namespace))
			Expect(name).To(Equal(serviceInstanceName))
		})

		When("the serviceInstance name is a duplicate", func() {
			BeforeEach(func() {
				duplicateValidator.ValidateCreateReturns(webhooks.ErrorDuplicateName)
			})

			It("denies the request", func() {
				Expect(response.Allowed).To(BeFalse())
				Expect(string(response.Result.Reason)).To(Equal(webhooks.ValidationError{Type: services.DuplicateServiceInstanceNameError, Message: `The service instance name is taken: ` + serviceInstance.Spec.Name}.Marshal()))
			})
		})

		When("there is an issue decoding the request", func() {
			BeforeEach(func() {
				cfAppJSON, err := json.Marshal(serviceInstance)
				Expect(err).NotTo(HaveOccurred())
				badCFServiceInstanceJSON := []byte("}" + string(cfAppJSON))

				request = admission.Request{
					AdmissionRequest: admissionv1.AdmissionRequest{
						Name:      serviceInstanceGUID,
						Namespace: defaultNamespace,
						Operation: admissionv1.Create,
						Object: runtime.RawExtension{
							Raw: badCFServiceInstanceJSON,
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

		When("validating the serviceInstance name fails", func() {
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
		var updatedServiceInstance *servicesv1alpha1.CFServiceInstance

		BeforeEach(func() {
			updatedServiceInstance = &servicesv1alpha1.CFServiceInstance{
				ObjectMeta: metav1.ObjectMeta{
					Name:      serviceInstanceGUID,
					Namespace: defaultNamespace,
				},
				Spec: servicesv1alpha1.CFServiceInstanceSpec{
					Name: "the-new-name",
					Type: servicesv1alpha1.UserProvidedType,
				},
			}
		})

		JustBeforeEach(func() {
			appJSON, err := json.Marshal(serviceInstance)
			Expect(err).NotTo(HaveOccurred())

			updatedAppJSON, err := json.Marshal(updatedServiceInstance)
			Expect(err).NotTo(HaveOccurred())

			request = admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Name:      serviceInstanceGUID,
					Namespace: defaultNamespace,
					Operation: admissionv1.Update,
					Object: runtime.RawExtension{
						Raw: updatedAppJSON,
					},
					OldObject: runtime.RawExtension{
						Raw: appJSON,
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
			Expect(namespace).To(Equal(serviceInstance.Namespace))
			Expect(oldName).To(Equal(serviceInstance.Spec.Name))
			Expect(newName).To(Equal(updatedServiceInstance.Spec.Name))
		})

		When("the new serviceInstance name is a duplicate", func() {
			BeforeEach(func() {
				duplicateValidator.ValidateUpdateReturns(webhooks.ErrorDuplicateName)
			})

			It("denies the request", func() {
				Expect(response.Allowed).To(BeFalse())
				Expect(string(response.Result.Reason)).To(Equal(webhooks.ValidationError{Type: services.DuplicateServiceInstanceNameError, Message: `The service instance name is taken: ` + updatedServiceInstance.Spec.Name}.Marshal()))
			})
		})

		When("the update validation fails for another reason", func() {
			BeforeEach(func() {
				duplicateValidator.ValidateUpdateReturns(errors.New("boom"))
			})

			It("denies the request", func() {
				Expect(response.Allowed).To(BeFalse())
				Expect(string(response.Result.Reason)).To(Equal(webhooks.AdmissionUnknownErrorReason()))
			})
		})
	})

	Describe("Delete", func() {
		JustBeforeEach(func() {
			appJSON, err := json.Marshal(serviceInstance)
			Expect(err).NotTo(HaveOccurred())

			request = admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Name:      serviceInstanceGUID,
					Namespace: defaultNamespace,
					Operation: admissionv1.Delete,
					OldObject: runtime.RawExtension{
						Raw: appJSON,
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
			Expect(namespace).To(Equal(serviceInstance.Namespace))
			Expect(name).To(Equal(serviceInstance.Spec.Name))
		})

		When("delete validation fails", func() {
			BeforeEach(func() {
				duplicateValidator.ValidateDeleteReturns(errors.New("boom"))
			})

			It("disallows the request", func() {
				Expect(response.Allowed).To(BeFalse())
				Expect(string(response.Result.Reason)).To(Equal(webhooks.AdmissionUnknownErrorReason()))
			})
		})
	})
})
