package services_test

import (
	"context"
	"errors"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	servicesv1alpha1 "code.cloudfoundry.org/cf-k8s-controllers/controllers/apis/services/v1alpha1"
	. "code.cloudfoundry.org/cf-k8s-controllers/controllers/controllers/services"
	"code.cloudfoundry.org/cf-k8s-controllers/controllers/controllers/services/fake"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("CFServiceInstance.Reconcile", func() {
	var (
		fakeClient       *fake.Client
		fakeStatusWriter *fake.StatusWriter

		cfServiceInstance       *servicesv1alpha1.CFServiceInstance
		cfServiceInstanceSecret *corev1.Secret

		getCFServiceInstanceError          error
		getCFServiceInstanceSecretError    error
		updateCFServiceInstanceStatusError error

		cfServiceInstanceReconciler *CFServiceInstanceReconciler
		ctx                         context.Context
		req                         ctrl.Request

		reconcileResult ctrl.Result
		reconcileErr    error
	)

	BeforeEach(func() {
		getCFServiceInstanceError = nil
		getCFServiceInstanceSecretError = nil
		updateCFServiceInstanceStatusError = nil

		fakeClient = new(fake.Client)
		fakeStatusWriter = new(fake.StatusWriter)
		fakeClient.StatusReturns(fakeStatusWriter)

		cfServiceInstance = new(servicesv1alpha1.CFServiceInstance)
		cfServiceInstanceSecret = new(corev1.Secret)

		fakeClient.GetStub = func(_ context.Context, _ types.NamespacedName, obj client.Object) error {
			switch obj := obj.(type) {
			case *servicesv1alpha1.CFServiceInstance:
				cfServiceInstance.DeepCopyInto(obj)
				return getCFServiceInstanceError
			case *corev1.Secret:
				cfServiceInstanceSecret.DeepCopyInto(obj)
				return getCFServiceInstanceSecretError
			default:
				panic("TestClient Get provided an unexpected object type")
			}
		}

		fakeStatusWriter.UpdateStub = func(ctx context.Context, obj client.Object, option ...client.UpdateOption) error {
			return updateCFServiceInstanceStatusError
		}

		Expect(servicesv1alpha1.AddToScheme(scheme.Scheme)).To(Succeed())

		cfServiceInstanceReconciler = &CFServiceInstanceReconciler{
			Client: fakeClient,
			Scheme: scheme.Scheme,
			Log:    zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)),
		}
		ctx = context.Background()
		req = ctrl.Request{
			NamespacedName: types.NamespacedName{
				Name:      "make-this-a-guid",
				Namespace: "make-this-a-guid-too",
			},
		}
	})

	JustBeforeEach(func() {
		reconcileResult, reconcileErr = cfServiceInstanceReconciler.Reconcile(ctx, req)
	})

	When("the CFServiceInstance is being created", func() {
		When("on the happy path", func() {
			It("returns an empty result and does not return error, also updates cfServiceInstance status", func() {
				Expect(reconcileResult).To(Equal(ctrl.Result{}))
				Expect(reconcileErr).NotTo(HaveOccurred())

				Expect(fakeStatusWriter.UpdateCallCount()).To(Equal(1))
				_, serviceInstanceObj, _ := fakeStatusWriter.UpdateArgsForCall(0)
				updatedCFServiceInstance, ok := serviceInstanceObj.(*servicesv1alpha1.CFServiceInstance)
				Expect(ok).To(BeTrue())
				Expect(updatedCFServiceInstance.Status.Binding.Name).To(Equal(cfServiceInstanceSecret.Name))
				Expect(updatedCFServiceInstance.Status.Conditions).To(ContainElement(MatchFields(IgnoreExtras, Fields{
					"Type":    Equal("BindingSecretAvailable"),
					"Status":  Equal(metav1.ConditionTrue),
					"Reason":  Equal("SecretFound"),
					"Message": Equal(""),
				})))
			})
		})
		When("the secret isn't found", func() {
			BeforeEach(func() {
				getCFServiceInstanceSecretError = apierrors.NewNotFound(schema.GroupResource{}, cfServiceInstanceSecret.Name)
			})

			It("requeues the request", func() {
				Expect(reconcileResult).To(Equal(ctrl.Result{RequeueAfter: 2 * time.Second}))
				Expect(reconcileErr).NotTo(HaveOccurred())

				Expect(fakeStatusWriter.UpdateCallCount()).To(Equal(1))
				_, serviceInstanceObj, _ := fakeStatusWriter.UpdateArgsForCall(0)
				updatedCFServiceInstance, ok := serviceInstanceObj.(*servicesv1alpha1.CFServiceInstance)
				Expect(ok).To(BeTrue())
				Expect(updatedCFServiceInstance.Status.Binding.Name).To(BeEmpty())
				Expect(updatedCFServiceInstance.Status.Conditions).To(ContainElement(MatchFields(IgnoreExtras, Fields{
					"Type":    Equal("BindingSecretAvailable"),
					"Status":  Equal(metav1.ConditionFalse),
					"Reason":  Equal("SecretNotFound"),
					"Message": Equal("Binding secret does not exist"),
				})))
			})
		})
		When("the API errors fetching the secret", func() {
			BeforeEach(func() {
				getCFServiceInstanceSecretError = errors.New("some random error")
			})

			It("errors, and updates status", func() {
				Expect(reconcileErr).To(HaveOccurred())

				Expect(fakeStatusWriter.UpdateCallCount()).To(Equal(1))
				_, serviceInstanceObj, _ := fakeStatusWriter.UpdateArgsForCall(0)
				updatedCFServiceInstance, ok := serviceInstanceObj.(*servicesv1alpha1.CFServiceInstance)
				Expect(ok).To(BeTrue())
				Expect(updatedCFServiceInstance.Status.Binding.Name).To(BeEmpty())
				Expect(updatedCFServiceInstance.Status.Conditions).To(ContainElement(MatchFields(IgnoreExtras, Fields{
					"Type":    Equal("BindingSecretAvailable"),
					"Status":  Equal(metav1.ConditionFalse),
					"Reason":  Equal("UnknownError"),
					"Message": Equal("Error occurred while fetching secret: " + getCFServiceInstanceSecretError.Error()),
				})))
			})
		})
		When("The API errors setting status on the CFServiceInstance", func() {
			BeforeEach(func() {
				updateCFServiceInstanceStatusError = errors.New("some random error")
			})

			It("errors", func() {
				Expect(reconcileErr).To(HaveOccurred())
			})
		})
	})
})
