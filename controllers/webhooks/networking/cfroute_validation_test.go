package networking_test

import (
	"context"
	"encoding/json"
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	networkingv1alpha1 "code.cloudfoundry.org/cf-k8s-controllers/controllers/apis/networking/v1alpha1"
	"code.cloudfoundry.org/cf-k8s-controllers/controllers/webhooks"
	"code.cloudfoundry.org/cf-k8s-controllers/controllers/webhooks/networking"
	"code.cloudfoundry.org/cf-k8s-controllers/controllers/webhooks/networking/fake"
	admissionv1 "k8s.io/api/admission/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

var _ = Describe("CF Route Validation", func() {
	var (
		ctx                context.Context
		duplicateValidator *fake.NameValidator
		realDecoder        *admission.Decoder
		cfRoute            *networkingv1alpha1.CFRoute
		request            admission.Request
		validatingWebhook  *networking.CFRouteValidation
		response           admission.Response
		cfRouteJSON        []byte

		testRouteGUID       string
		testRouteNamespace  string
		testRouteHost       string
		testRoutePath       string
		testDomainGUID      string
		testDomainNamespace string
		rootNamespace       string
	)

	BeforeEach(func() {
		ctx = context.Background()

		scheme := runtime.NewScheme()
		err := networkingv1alpha1.AddToScheme(scheme)
		Expect(err).NotTo(HaveOccurred())

		realDecoder, err = admission.NewDecoder(scheme)
		Expect(err).NotTo(HaveOccurred())

		testRouteGUID = "my-guid"
		testRouteNamespace = "my-ns"
		testRouteHost = "my-host"
		testRoutePath = "my-path"
		testDomainGUID = "domain-guid"
		testDomainNamespace = "domain-ns"
		rootNamespace = "root-ns"

		cfRoute = &networkingv1alpha1.CFRoute{
			ObjectMeta: metav1.ObjectMeta{
				Name:      testRouteGUID,
				Namespace: testRouteNamespace,
			},
			Spec: networkingv1alpha1.CFRouteSpec{
				Host:     testRouteHost,
				Path:     testRoutePath,
				Protocol: "http",
				DomainRef: v1.ObjectReference{
					Name:      testDomainGUID,
					Namespace: testDomainNamespace,
				},
			},
		}

		cfRouteJSON, err = json.Marshal(cfRoute)
		Expect(err).NotTo(HaveOccurred())

		duplicateValidator = new(fake.NameValidator)
		validatingWebhook = networking.NewCFRouteValidation(duplicateValidator, rootNamespace)

		Expect(validatingWebhook.InjectDecoder(realDecoder)).To(Succeed())
	})

	Describe("Create", func() {
		JustBeforeEach(func() {
			response = validatingWebhook.Handle(ctx, request)
		})

		BeforeEach(func() {
			request = admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Name:      testRouteGUID,
					Namespace: testRouteNamespace,
					Operation: admissionv1.Create,
					Object: runtime.RawExtension{
						Raw: cfRouteJSON,
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
			Expect(name).To(Equal(testRouteHost + "::" + testDomainNamespace + "::" + testDomainGUID + "::" + testRoutePath))
		})

		When("the app name is a duplicate", func() {
			BeforeEach(func() {
				duplicateValidator.ValidateCreateReturns(webhooks.ErrorDuplicateName)
			})

			It("denies the request", func() {
				Expect(response.Allowed).To(BeFalse())
				Expect(string(response.Result.Reason)).To(Equal(webhooks.DuplicateRouteError.Marshal()))
			})
		})

		When("there is an issue decoding the request", func() {
			BeforeEach(func() {
				var err error
				cfRouteJSON, err = json.Marshal(cfRoute)
				Expect(err).NotTo(HaveOccurred())
				badCFAppJSON := []byte("}" + string(cfRouteJSON))

				request = admission.Request{
					AdmissionRequest: admissionv1.AdmissionRequest{
						Name:      testRouteGUID,
						Namespace: testRouteNamespace,
						Operation: admissionv1.Create,
						Object: runtime.RawExtension{
							Raw: badCFAppJSON,
						},
					},
				}
			})

			It("denies the request", func() {
				Expect(response.Allowed).To(BeFalse())
			})

			It("does not attempt to validate a name", func() {
				Expect(duplicateValidator.ValidateCreateCallCount()).To(Equal(0))
			})
		})

		When("validating the app name fails", func() {
			BeforeEach(func() {
				duplicateValidator.ValidateCreateReturns(errors.New("boom"))
			})

			It("denies the request", func() {
				Expect(response.Allowed).To(BeFalse())
				Expect(string(response.Result.Reason)).To(Equal(webhooks.UnknownError.Marshal()))
			})
		})

		When("host is empty on the route", func() {
			BeforeEach(func() {
				var err error
				cfRoute.Spec.Host = ""
				cfRouteJSON, err = json.Marshal(cfRoute)
				Expect(err).NotTo(HaveOccurred())

				request = admission.Request{
					AdmissionRequest: admissionv1.AdmissionRequest{
						Name:      testRouteGUID,
						Namespace: testRouteNamespace,
						Operation: admissionv1.Create,
						Object: runtime.RawExtension{
							Raw: cfRouteJSON,
						},
					},
				}
			})

			It("denies the request", func() {
				Expect(response.Allowed).To(BeFalse())
			})
		})
	})

	Describe("Update", func() {
		var (
			updatedCFRoute   *networkingv1alpha1.CFRoute
			newTestRoutePath string
		)

		BeforeEach(func() {
			newTestRoutePath = "new-path"
			updatedCFRoute = &networkingv1alpha1.CFRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testRouteGUID,
					Namespace: testRouteNamespace,
				},
				Spec: networkingv1alpha1.CFRouteSpec{
					Host:     testRouteHost,
					Path:     newTestRoutePath,
					Protocol: "http",
					DomainRef: v1.ObjectReference{
						Name:      testDomainGUID,
						Namespace: testDomainNamespace,
					},
				},
			}
		})

		JustBeforeEach(func() {
			routeJSON, err := json.Marshal(cfRoute)
			Expect(err).NotTo(HaveOccurred())

			updatedRouteJSON, err := json.Marshal(updatedCFRoute)
			Expect(err).NotTo(HaveOccurred())

			request = admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Name:      testRouteGUID,
					Namespace: testRouteNamespace,
					Operation: admissionv1.Update,
					Object: runtime.RawExtension{
						Raw: updatedRouteJSON,
					},
					OldObject: runtime.RawExtension{
						Raw: routeJSON,
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
			Expect(namespace).To(Equal(rootNamespace))
			Expect(oldName).To(Equal(testRouteHost + "::" + testDomainNamespace + "::" + testDomainGUID + "::" + testRoutePath))
			Expect(newName).To(Equal(testRouteHost + "::" + testDomainNamespace + "::" + testDomainGUID + "::" + newTestRoutePath))
		})

		When("the new app name is a duplicate", func() {
			BeforeEach(func() {
				duplicateValidator.ValidateUpdateReturns(webhooks.ErrorDuplicateName)
			})

			It("denies the request", func() {
				Expect(response.Allowed).To(BeFalse())
				Expect(string(response.Result.Reason)).To(Equal(webhooks.DuplicateRouteError.Marshal()))
			})
		})

		When("the update validation fails for another reason", func() {
			BeforeEach(func() {
				duplicateValidator.ValidateUpdateReturns(errors.New("boom!"))
			})

			It("denies the request", func() {
				Expect(response.Allowed).To(BeFalse())
				Expect(string(response.Result.Reason)).To(Equal(webhooks.UnknownError.Marshal()))
			})
		})

		When("the new hostname is empty on the route", func() {
			BeforeEach(func() {
				updatedCFRoute.Spec.Host = ""
			})

			It("denies the request", func() {
				Expect(response.Allowed).To(BeFalse())
			})
		})
	})

	Describe("Delete", func() {
		JustBeforeEach(func() {
			routeJSON, err := json.Marshal(cfRoute)
			Expect(err).NotTo(HaveOccurred())

			request = admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Name:      testRouteGUID,
					Namespace: testRouteNamespace,
					Operation: admissionv1.Delete,
					OldObject: runtime.RawExtension{
						Raw: routeJSON,
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
			Expect(namespace).To(Equal(rootNamespace))
			Expect(name).To(Equal(testRouteHost + "::" + testDomainNamespace + "::" + testDomainGUID + "::" + testRoutePath))
		})

		When("delete validation fails", func() {
			BeforeEach(func() {
				duplicateValidator.ValidateDeleteReturns(errors.New("boom!"))
			})

			It("disallows the request", func() {
				Expect(response.Allowed).To(BeFalse())
				Expect(string(response.Result.Reason)).To(Equal(webhooks.UnknownError.Marshal()))
			})
		})
	})
})
