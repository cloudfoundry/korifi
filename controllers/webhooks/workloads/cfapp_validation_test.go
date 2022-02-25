package workloads_test

import (
	"context"
	"encoding/json"
	"errors"

	workloadsv1alpha1 "code.cloudfoundry.org/cf-k8s-controllers/controllers/apis/workloads/v1alpha1"
	"code.cloudfoundry.org/cf-k8s-controllers/controllers/webhooks/workloads"
	"code.cloudfoundry.org/cf-k8s-controllers/controllers/webhooks/workloads/fake"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	admissionv1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

var _ = Describe("CFAppValidatingWebhook", func() {
	const (
		testAppGUID      = "test-app-guid"
		testAppName      = "test-app"
		testAppNamespace = "default"
	)

	var (
		ctx                context.Context
		duplicateValidator *fake.NameValidator
		realDecoder        *admission.Decoder
		app                *workloadsv1alpha1.CFApp
		request            admission.Request
		validatingWebhook  *workloads.CFAppValidation
		response           admission.Response
		cfAppJSON          []byte
	)

	BeforeEach(func() {
		ctx = context.Background()

		scheme := runtime.NewScheme()
		err := workloadsv1alpha1.AddToScheme(scheme)
		Expect(err).NotTo(HaveOccurred())

		realDecoder, err = admission.NewDecoder(scheme)
		Expect(err).NotTo(HaveOccurred())

		app = &workloadsv1alpha1.CFApp{
			ObjectMeta: metav1.ObjectMeta{
				Name:      testAppGUID,
				Namespace: testAppNamespace,
			},
			Spec: workloadsv1alpha1.CFAppSpec{
				Name:         testAppName,
				DesiredState: workloadsv1alpha1.StoppedState,
			},
		}

		cfAppJSON, err = json.Marshal(app)
		Expect(err).NotTo(HaveOccurred())

		duplicateValidator = new(fake.NameValidator)
		validatingWebhook = workloads.NewCFAppValidation(duplicateValidator)

		Expect(validatingWebhook.InjectDecoder(realDecoder)).To(Succeed())
	})

	Describe("Create", func() {
		JustBeforeEach(func() {
			response = validatingWebhook.Handle(ctx, request)
		})

		BeforeEach(func() {
			request = admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Name:      testAppGUID,
					Namespace: testAppNamespace,
					Operation: admissionv1.Create,
					Object: runtime.RawExtension{
						Raw: cfAppJSON,
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
			Expect(namespace).To(Equal(testAppNamespace))
			Expect(name).To(Equal(testAppName))
		})

		When("the app name is a duplicate", func() {
			BeforeEach(func() {
				duplicateValidator.ValidateCreateReturns(workloads.ErrorDuplicateName)
			})

			It("denies the request", func() {
				Expect(response.Allowed).To(BeFalse())
				Expect(string(response.Result.Reason)).To(Equal(workloads.DuplicateAppError.Marshal()))
			})
		})

		When("there is an issue decoding the request", func() {
			BeforeEach(func() {
				cfAppJSON, err := json.Marshal(app)
				Expect(err).NotTo(HaveOccurred())
				badCFAppJSON := []byte("}" + string(cfAppJSON))

				request = admission.Request{
					AdmissionRequest: admissionv1.AdmissionRequest{
						Name:      testAppGUID,
						Namespace: testAppNamespace,
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

			It("does not attempt to register a name", func() {
				Expect(duplicateValidator.ValidateCreateCallCount()).To(Equal(0))
			})
		})

		When("validating the app name fails", func() {
			BeforeEach(func() {
				duplicateValidator.ValidateCreateReturns(errors.New("boom"))
			})

			It("denies the request", func() {
				Expect(response.Allowed).To(BeFalse())
				Expect(string(response.Result.Reason)).To(Equal(workloads.UnknownError.Marshal()))
			})
		})
	})

	Describe("Update", func() {
		var updatedApp *workloadsv1alpha1.CFApp

		BeforeEach(func() {
			updatedApp = &workloadsv1alpha1.CFApp{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testAppGUID,
					Namespace: testAppNamespace,
				},
				Spec: workloadsv1alpha1.CFAppSpec{
					Name:         "the-new-name",
					DesiredState: workloadsv1alpha1.StoppedState,
				},
			}
		})

		JustBeforeEach(func() {
			appJSON, err := json.Marshal(app)
			Expect(err).NotTo(HaveOccurred())

			updatedAppJSON, err := json.Marshal(updatedApp)
			Expect(err).NotTo(HaveOccurred())

			request = admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Name:      testAppGUID,
					Namespace: testAppNamespace,
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
			Expect(namespace).To(Equal(app.Namespace))
			Expect(oldName).To(Equal(app.Spec.Name))
			Expect(newName).To(Equal(updatedApp.Spec.Name))
		})

		When("the new app name is a duplicate", func() {
			BeforeEach(func() {
				duplicateValidator.ValidateUpdateReturns(workloads.ErrorDuplicateName)
			})

			It("denies the request", func() {
				Expect(response.Allowed).To(BeFalse())
				Expect(string(response.Result.Reason)).To(Equal(workloads.DuplicateAppError.Marshal()))
			})
		})

		When("the update validation fails for another reason", func() {
			BeforeEach(func() {
				duplicateValidator.ValidateUpdateReturns(errors.New("boom!"))
			})

			It("denies the request", func() {
				Expect(response.Allowed).To(BeFalse())
				Expect(string(response.Result.Reason)).To(Equal(workloads.UnknownError.Marshal()))
			})
		})
	})

	Describe("Delete", func() {
		JustBeforeEach(func() {
			appJSON, err := json.Marshal(app)
			Expect(err).NotTo(HaveOccurred())

			request = admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Name:      testAppGUID,
					Namespace: testAppNamespace,
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
			Expect(namespace).To(Equal(app.Namespace))
			Expect(name).To(Equal(app.Spec.Name))
		})

		When("delete validation fails", func() {
			BeforeEach(func() {
				duplicateValidator.ValidateDeleteReturns(errors.New("boom!"))
			})

			It("disallows the request", func() {
				Expect(response.Allowed).To(BeFalse())
				Expect(string(response.Result.Reason)).To(Equal(workloads.UnknownError.Marshal()))
			})
		})
	})
})
