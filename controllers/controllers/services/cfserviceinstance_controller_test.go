package services_test

import (
	"context"
	"errors"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	. "code.cloudfoundry.org/korifi/controllers/controllers/services"
	"code.cloudfoundry.org/korifi/controllers/fake"

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

		cfServiceInstance       *korifiv1alpha1.CFServiceInstance
		cfServiceInstanceSecret *corev1.Secret

		getCFServiceInstanceError         error
		getCFServiceInstanceSecretError   error
		patchCFServiceInstanceStatusError error

		cfServiceInstanceReconciler *CFServiceInstanceReconciler
		ctx                         context.Context
		req                         ctrl.Request

		reconcileResult ctrl.Result
		reconcileErr    error
	)

	BeforeEach(func() {
		getCFServiceInstanceError = nil
		getCFServiceInstanceSecretError = nil
		patchCFServiceInstanceStatusError = nil

		fakeClient = new(fake.Client)
		fakeStatusWriter = new(fake.StatusWriter)
		fakeClient.StatusReturns(fakeStatusWriter)

		cfServiceInstance = new(korifiv1alpha1.CFServiceInstance)
		cfServiceInstanceSecret = new(corev1.Secret)

		fakeClient.GetStub = func(_ context.Context, _ types.NamespacedName, obj client.Object, _ ...client.GetOption) error {
			switch obj := obj.(type) {
			case *korifiv1alpha1.CFServiceInstance:
				cfServiceInstance.DeepCopyInto(obj)
				return getCFServiceInstanceError
			case *corev1.Secret:
				cfServiceInstanceSecret.DeepCopyInto(obj)
				return getCFServiceInstanceSecretError
			default:
				panic("TestClient Get provided an unexpected object type")
			}
		}

		fakeStatusWriter.PatchStub = func(ctx context.Context, obj client.Object, patch client.Patch, option ...client.PatchOption) error {
			return patchCFServiceInstanceStatusError
		}

		Expect(korifiv1alpha1.AddToScheme(scheme.Scheme)).To(Succeed())

		cfServiceInstanceReconciler = NewCFServiceInstanceReconciler(
			fakeClient,
			scheme.Scheme,
			zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)),
		)
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

				Expect(fakeStatusWriter.PatchCallCount()).To(Equal(1))
				_, serviceInstanceObj, _, _ := fakeStatusWriter.PatchArgsForCall(0)
				updatedCFServiceInstance, ok := serviceInstanceObj.(*korifiv1alpha1.CFServiceInstance)
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

				Expect(fakeStatusWriter.PatchCallCount()).To(Equal(1))
				_, serviceInstanceObj, _, _ := fakeStatusWriter.PatchArgsForCall(0)
				updatedCFServiceInstance, ok := serviceInstanceObj.(*korifiv1alpha1.CFServiceInstance)
				Expect(ok).To(BeTrue())

				Expect(updatedCFServiceInstance.Status.Binding).To(BeZero())
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

				Expect(fakeStatusWriter.PatchCallCount()).To(Equal(1))
				_, serviceInstanceObj, _, _ := fakeStatusWriter.PatchArgsForCall(0)
				updatedCFServiceInstance, ok := serviceInstanceObj.(*korifiv1alpha1.CFServiceInstance)
				Expect(ok).To(BeTrue())

				Expect(updatedCFServiceInstance.Status.Binding).To(BeZero())
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
				patchCFServiceInstanceStatusError = errors.New("some random error")
			})

			It("errors", func() {
				Expect(reconcileErr).To(HaveOccurred())
			})
		})
		When("adding the finalizer to the CFRoute returns an error", func() {
			BeforeEach(func() {
				fakeClient.PatchReturns(errors.New("failed to patch CFServiceInstance"))
			})

			It("returns the error", func() {
				Expect(reconcileErr).To(MatchError("failed to patch CFServiceInstance"))
			})
		})
	})

	When("the CFServiceInstance is being deleted", func() {
		var (
			cfServiceBindingList    korifiv1alpha1.CFServiceBindingList
			listCFServiceBindingErr error
		)
		BeforeEach(func() {
			cfServiceInstance = &korifiv1alpha1.CFServiceInstance{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "",
					Namespace: "",
					Finalizers: []string{
						"cfServiceInstance.korifi.cloudfoundry.org",
					},
					DeletionTimestamp: &metav1.Time{
						Time: time.Now(),
					},
				},
				Spec: korifiv1alpha1.CFServiceInstanceSpec{
					DisplayName: "",
					SecretName:  "",
					Type:        "",
				},
			}

			cfServiceBindingList = korifiv1alpha1.CFServiceBindingList{
				Items: []korifiv1alpha1.CFServiceBinding{{}},
			}
			listCFServiceBindingErr = nil

			fakeClient.ListStub = func(ctx context.Context, list client.ObjectList, option ...client.ListOption) error {
				switch list := list.(type) {
				case *korifiv1alpha1.CFServiceBindingList:
					cfServiceBindingList.DeepCopyInto(list)
					return listCFServiceBindingErr
				default:
					panic("TestClient List provided a weird obj")
				}
			}
		})
		When("listing the associated CFServiceBindings fails", func() {
			BeforeEach(func() {
				listCFServiceBindingErr = errors.New("fail list on purpose")
			})
			It("returns the error", func() {
				Expect(reconcileErr).To(MatchError(listCFServiceBindingErr))
			})
		})
		When("delete the CFServiceBinding fails", func() {
			BeforeEach(func() {
				fakeClient.DeleteReturns(errors.New("delete service binding failed"))
			})
			It("returns the error", func() {
				Expect(reconcileErr).To(MatchError("delete service binding failed"))
			})
		})

		When("removing the finalizer from the CFRoute fails", func() {
			BeforeEach(func() {
				fakeClient.PatchReturns(errors.New("failed to patch CFServiceInstance"))
			})

			It("returns the error", func() {
				Expect(reconcileErr).To(MatchError("failed to patch CFServiceInstance"))
			})
		})
	})
})
