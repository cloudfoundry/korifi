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
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

var _ = Describe("CFAppValidatingWebhook", func() {
	const (
		testAppGUID      = "test-app-guid"
		testAppName      = "test-app"
		testAppNamespace = "default"
	)

	var (
		ctx               context.Context
		nameRegistry      *fake.NameRegistry
		realDecoder       *admission.Decoder
		app               *workloadsv1alpha1.CFApp
		request           admission.Request
		validatingWebhook *workloads.CFAppValidation
		response          admission.Response
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

		cfAppJSON, err := json.Marshal(app)
		Expect(err).NotTo(HaveOccurred())

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

		nameRegistry = new(fake.NameRegistry)
		validatingWebhook = workloads.NewCFAppValidation(nameRegistry)

		Expect(validatingWebhook.InjectDecoder(realDecoder)).To(Succeed())
	})

	Describe("Create", func() {
		JustBeforeEach(func() {
			response = validatingWebhook.Handle(ctx, request)
		})

		It("allows the request", func() {
			Expect(response.Allowed).To(BeTrue())
		})

		It("uses the name registry to register the name", func() {
			Expect(nameRegistry.RegisterNameCallCount()).To(Equal(1))
			actualContext, namespace, name := nameRegistry.RegisterNameArgsForCall(0)
			Expect(actualContext).To(Equal(ctx))
			Expect(namespace).To(Equal(testAppNamespace))
			Expect(name).To(Equal(testAppName))
		})

		When("the app name is already registered", func() {
			BeforeEach(func() {
				nameRegistry.RegisterNameReturns(k8serrors.NewAlreadyExists(schema.GroupResource{}, "jim"))
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
				Expect(nameRegistry.RegisterNameCallCount()).To(Equal(0))
			})
		})

		When("registering the app name fails", func() {
			BeforeEach(func() {
				nameRegistry.RegisterNameReturns(errors.New("boom"))
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

		It("locks the old name", func() {
			Expect(nameRegistry.TryLockNameCallCount()).To(Equal(1))
			_, namespace, name := nameRegistry.TryLockNameArgsForCall(0)
			Expect(namespace).To(Equal(app.Namespace))
			Expect(name).To(Equal(app.Spec.Name))
		})

		It("registers the new name", func() {
			Expect(nameRegistry.RegisterNameCallCount()).To(Equal(1))
			_, namespace, name := nameRegistry.RegisterNameArgsForCall(0)
			Expect(namespace).To(Equal(updatedApp.Namespace))
			Expect(name).To(Equal(updatedApp.Spec.Name))
		})

		It("deregisters the old name", func() {
			Expect(nameRegistry.DeregisterNameCallCount()).To(Equal(1))
			_, namespace, name := nameRegistry.DeregisterNameArgsForCall(0)
			Expect(namespace).To(Equal(app.Namespace))
			Expect(name).To(Equal(app.Spec.Name))
		})

		When("the name isn't changed", func() {
			BeforeEach(func() {
				updatedApp.Spec.DesiredState = workloadsv1alpha1.StartedState
				updatedApp.Spec.Name = testAppName
			})

			It("is allowed without using the name registry", func() {
				Expect(response.Allowed).To(BeTrue())
				Expect(nameRegistry.TryLockNameCallCount()).To(Equal(0))
				Expect(nameRegistry.RegisterNameCallCount()).To(Equal(0))
				Expect(nameRegistry.DeregisterNameCallCount()).To(Equal(0))
			})
		})

		When("taking the lock on the old name fails", func() {
			BeforeEach(func() {
				nameRegistry.TryLockNameReturns(errors.New("nope"))
			})

			It("denies the request", func() {
				Expect(response.Allowed).To(BeFalse())
				Expect(string(response.Result.Reason)).To(Equal(workloads.UnknownError.Marshal()))
			})

			It("does not register the new name", func() {
				Expect(nameRegistry.RegisterNameCallCount()).To(Equal(0))
			})
		})

		When("the new name is already used", func() {
			BeforeEach(func() {
				nameRegistry.RegisterNameReturns(k8serrors.NewAlreadyExists(schema.GroupResource{}, "foo"))
			})

			It("denies the request", func() {
				Expect(response.Allowed).To(BeFalse())
				Expect(string(response.Result.Reason)).To(Equal(workloads.DuplicateAppError.Marshal()))
			})
		})

		When("registering the new name fails", func() {
			BeforeEach(func() {
				nameRegistry.RegisterNameReturns(errors.New("boom"))
			})

			It("denies the request", func() {
				Expect(response.Allowed).To(BeFalse())
				Expect(string(response.Result.Reason)).To(Equal(workloads.UnknownError.Marshal()))
			})

			It("releases the lock on the old name", func() {
				Expect(nameRegistry.UnlockNameCallCount()).To(Equal(1))
				_, namespace, name := nameRegistry.UnlockNameArgsForCall(0)
				Expect(namespace).To(Equal(app.Namespace))
				Expect(name).To(Equal(app.Spec.Name))
			})

			When("releasing the old name lock fails", func() {
				BeforeEach(func() {
					nameRegistry.UnlockNameReturns(errors.New("oops"))
				})

				It("continues and denies the request", func() {
					Expect(response.Allowed).To(BeFalse())
					Expect(string(response.Result.Reason)).To(Equal(workloads.UnknownError.Marshal()))
				})
			})
		})

		When("releasing the old name fails", func() {
			BeforeEach(func() {
				nameRegistry.DeregisterNameReturns(errors.New("boom"))
			})

			It("continues anyway and allows the request", func() {
				Expect(response.Allowed).To(BeTrue())
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

		It("deregisters the old name", func() {
			Expect(nameRegistry.DeregisterNameCallCount()).To(Equal(1))
			_, namespace, name := nameRegistry.DeregisterNameArgsForCall(0)
			Expect(namespace).To(Equal(app.Namespace))
			Expect(name).To(Equal(app.Spec.Name))
		})

		When("the old name doesn't exist", func() {
			BeforeEach(func() {
				nameRegistry.DeregisterNameReturns(k8serrors.NewNotFound(schema.GroupResource{}, "foo"))
			})

			It("allows the request", func() {
				Expect(response.Allowed).To(BeTrue())
			})
		})

		When("deregistering fails", func() {
			BeforeEach(func() {
				nameRegistry.DeregisterNameReturns(errors.New("boom"))
			})

			It("disallows the request", func() {
				Expect(response.Allowed).To(BeFalse())
				Expect(string(response.Result.Reason)).To(Equal(workloads.UnknownError.Marshal()))
			})
		})
	})
})
