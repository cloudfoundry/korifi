package services_test

import (
	"context"
	"errors"
	"fmt"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	. "code.cloudfoundry.org/korifi/controllers/controllers/services"
	servicesfake "code.cloudfoundry.org/korifi/controllers/controllers/services/fake"
	"code.cloudfoundry.org/korifi/tools/k8s"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	servicebindingv1beta1 "github.com/servicebinding/service-binding-controller/apis/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("CFServiceBinding.Reconcile", func() {
	var (
		fakeBuilder *servicesfake.VCAPServicesSecretBuilder

		cfServiceBinding        *korifiv1alpha1.CFServiceBinding
		cfServiceInstance       *korifiv1alpha1.CFServiceInstance
		cfServiceInstanceSecret *corev1.Secret
		sbServiceBinding        *servicebindingv1beta1.ServiceBinding
		cfApp                   *korifiv1alpha1.CFApp
		cfAppStatus             korifiv1alpha1.CFAppStatus

		getCFServiceBindingError          error
		getCFServiceInstanceSecretError   error
		updateCFServiceBindingStatusError error
		getCFServiceInstanceError         error
		getCFAppError                     error
		patchCFServiceBindingError        error

		getSBServiceBindingError   error
		patchSBServiceBindingError error
		patchSecretError           error

		cfServiceBindingReconciler *k8s.PatchingReconciler[korifiv1alpha1.CFServiceBinding, *korifiv1alpha1.CFServiceBinding]
		ctx                        context.Context
		req                        ctrl.Request

		cfAppName             string
		cfServiceInstanceName string
		secretType            string
		secretProvider        string

		reconcileResult ctrl.Result
		reconcileErr    error
	)

	BeforeEach(func() {
		getCFServiceBindingError = nil
		getCFServiceInstanceSecretError = nil
		getCFServiceInstanceError = nil
		getCFAppError = nil
		updateCFServiceBindingStatusError = nil
		patchCFServiceBindingError = nil

		getSBServiceBindingError = nil
		patchSBServiceBindingError = nil
		patchSecretError = nil

		cfAppName = "cfAppName"
		cfServiceInstanceName = "cfServiceInstanceName"
		secretType = "secretType"
		secretProvider = "secretProvider"

		fakeBuilder = new(servicesfake.VCAPServicesSecretBuilder)

		cfServiceBinding = new(korifiv1alpha1.CFServiceBinding)
		cfServiceInstance = new(korifiv1alpha1.CFServiceInstance)
		cfServiceInstanceSecret = new(corev1.Secret)
		sbServiceBinding = new(servicebindingv1beta1.ServiceBinding)
		cfApp = new(korifiv1alpha1.CFApp)
		cfAppStatus = korifiv1alpha1.CFAppStatus{
			VCAPServicesSecretName: cfAppName + "-vcap-services",
		}

		fakeClient.GetStub = func(_ context.Context, ref types.NamespacedName, obj client.Object, _ ...client.GetOption) error {
			switch obj := obj.(type) {
			case *korifiv1alpha1.CFServiceBinding:
				cfServiceBinding.DeepCopyInto(obj)
				return getCFServiceBindingError
			case *korifiv1alpha1.CFServiceInstance:
				cfServiceInstance.Name = cfServiceInstanceName
				cfServiceInstance.DeepCopyInto(obj)
				return getCFServiceInstanceError
			case *servicebindingv1beta1.ServiceBinding:
				if getSBServiceBindingError == nil {
					sbServiceBinding.DeepCopyInto(obj)
				}
				return getSBServiceBindingError
			case *korifiv1alpha1.CFApp:
				cfApp.Name = cfAppName
				cfApp.Status = cfAppStatus
				cfApp.DeepCopyInto(obj)
				return getCFAppError
			case *corev1.Secret:
				if ref.Name == "unfindable" {
					return apierrors.NewNotFound(schema.GroupResource{}, cfApp.Status.VCAPServicesSecretName)
				}
				cfServiceInstanceSecret.DeepCopyInto(obj)
				return getCFServiceInstanceSecretError
			default:
				panic("TestClient Get provided an unexpected object type")
			}
		}

		fakeStatusWriter.PatchStub = func(ctx context.Context, obj client.Object, patch client.Patch, option ...client.PatchOption) error {
			return updateCFServiceBindingStatusError
		}

		fakeClient.PatchStub = func(ctx context.Context, obj client.Object, patch client.Patch, option ...client.PatchOption) error {
			switch obj := obj.(type) {
			case *korifiv1alpha1.CFServiceBinding:
				cfServiceBinding.DeepCopyInto(obj)
				return patchCFServiceBindingError
			case *servicebindingv1beta1.ServiceBinding:
				sbServiceBinding.DeepCopyInto(obj)
				return patchSBServiceBindingError
			case *corev1.Secret:
				return patchSecretError
			default:
				panic("TestClient Patch provided an unexpected object type")
			}
		}

		Expect(korifiv1alpha1.AddToScheme(scheme.Scheme)).To(Succeed())
		Expect(servicebindingv1beta1.AddToScheme(scheme.Scheme)).To(Succeed())

		cfServiceBindingReconciler = NewCFServiceBindingReconciler(
			fakeClient,
			scheme.Scheme,
			zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)),
			fakeBuilder,
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
		reconcileResult, reconcileErr = cfServiceBindingReconciler.Reconcile(ctx, req)
	})

	When("the CFServiceBinding is being created", func() {
		When("on the happy path", func() {
			When("no servicebinding.io ServiceBinding exists", func() {
				BeforeEach(func() {
					getSBServiceBindingError = apierrors.NewNotFound(
						schema.GroupResource{Group: "servicebinding.io", Resource: "ServiceBinding"},
						"foo",
					)
				})
				It("returns an empty result and does not return error, also updates cfServiceBinding status", func() {
					Expect(reconcileResult).To(Equal(ctrl.Result{}))
					Expect(reconcileErr).NotTo(HaveOccurred())

					Expect(fakeStatusWriter.PatchCallCount()).To(Equal(1))
					_, serviceBindingObj, _, _ := fakeStatusWriter.PatchArgsForCall(0)
					updatedCFServiceBinding, ok := serviceBindingObj.(*korifiv1alpha1.CFServiceBinding)
					Expect(ok).To(BeTrue())
					Expect(updatedCFServiceBinding.Status.Binding.Name).To(Equal(cfServiceInstanceSecret.Name))
					Expect(updatedCFServiceBinding.Status.Conditions).To(ContainElement(MatchFields(IgnoreExtras, Fields{
						"Type":    Equal("BindingSecretAvailable"),
						"Status":  Equal(metav1.ConditionTrue),
						"Reason":  Equal("SecretFound"),
						"Message": Equal(""),
					})))
				})
				When("the secret has a provider and type", func() {
					BeforeEach(func() {
						cfServiceInstanceSecret.Data = map[string][]byte{
							"type":     []byte(secretType),
							"provider": []byte(secretProvider),
						}
					})
					It("it creates a servicebinding.io ServiceBinding with the type/provider filled in", func() {
						Expect(fakeClient.CreateCallCount()).To(Equal(1), "Client.Create call count mismatch")
						Expect(fakeClient.PatchCallCount()).To(Equal(2), "Client.Patch call count mismatch")
						_, returnedObj, _ := fakeClient.CreateArgsForCall(0)
						serviceBinding := returnedObj.(*servicebindingv1beta1.ServiceBinding)
						Expect(serviceBinding.Spec.Name).To(Equal(cfServiceInstanceSecret.Name))
						Expect(serviceBinding.Spec.Type).To(Equal(secretType))
						Expect(serviceBinding.Spec.Provider).To(Equal(secretProvider))
					})
				})
				When("the secret does not have a provider and type", func() {
					It("it creates a servicebinding.io ServiceBinding with a default type and no provider", func() {
						Expect(fakeClient.CreateCallCount()).To(Equal(1), "Client.Create call count mismatch")
						Expect(fakeClient.PatchCallCount()).To(Equal(2), "Client.Patch call count mismatch")
						_, returnedObj, _ := fakeClient.CreateArgsForCall(0)
						serviceBinding := returnedObj.(*servicebindingv1beta1.ServiceBinding)
						Expect(serviceBinding.Spec.Name).To(Equal(cfServiceInstanceSecret.Name))
						Expect(serviceBinding.Spec.Type).To(Equal("user-provided"))
						Expect(serviceBinding.Spec.Provider).To(Equal(""))
					})
				})
			})
			When("a servicebinding.io ServiceBinding exists", func() {
				BeforeEach(func() {
					sbServiceBinding = &servicebindingv1beta1.ServiceBinding{
						ObjectMeta: metav1.ObjectMeta{
							Name:      fmt.Sprintf("cf-binding-%s", cfServiceBinding.Name),
							Namespace: cfServiceBinding.Namespace,
						},
					}
				})
				It("returns an empty result and does not return error, also updates cfServiceBinding status", func() {
					Expect(reconcileResult).To(Equal(ctrl.Result{}))
					Expect(reconcileErr).NotTo(HaveOccurred())

					Expect(fakeStatusWriter.PatchCallCount()).To(Equal(1))
					_, serviceBindingObj, _, _ := fakeStatusWriter.PatchArgsForCall(0)
					updatedCFServiceBinding, ok := serviceBindingObj.(*korifiv1alpha1.CFServiceBinding)
					Expect(ok).To(BeTrue())
					Expect(updatedCFServiceBinding.Status.Binding.Name).To(Equal(cfServiceInstanceSecret.Name))
					Expect(updatedCFServiceBinding.Status.Conditions).To(ContainElement(MatchFields(IgnoreExtras, Fields{
						"Type":    Equal("BindingSecretAvailable"),
						"Status":  Equal(metav1.ConditionTrue),
						"Reason":  Equal("SecretFound"),
						"Message": Equal(""),
					})))
				})
				It("patches the existing servicebinding.io ServiceBinding", func() {
					Expect(fakeClient.CreateCallCount()).To(Equal(0), "Client.Create call count mismatch")
					Expect(fakeClient.PatchCallCount()).To(Equal(3), "Client.Patch call count mismatch")
				})
				It("patches the existing VCAP Services Secret", func() {
					Expect(fakeBuilder.BuildVCAPServicesEnvValueCallCount()).To(Equal(1))
					_, appArg := fakeBuilder.BuildVCAPServicesEnvValueArgsForCall(0)
					Expect(appArg.Name).To(Equal(cfAppName))
				})
			})
		})
		When("the app isn't found", func() {
			BeforeEach(func() {
				getCFAppError = apierrors.NewNotFound(schema.GroupResource{}, cfApp.Name)
			})
			It("does not return an error", func() {
				Expect(reconcileResult).To(Equal(ctrl.Result{}))
				Expect(reconcileErr).NotTo(HaveOccurred())
			})
		})
		When("the API errors setting the ownerReference", func() {
			BeforeEach(func() {
				patchCFServiceBindingError = errors.New("some random error")
			})
			It("returns an error", func() {
				Expect(reconcileErr).To(MatchError(patchCFServiceBindingError))
			})
		})
		When("the instance isn't found", func() {
			BeforeEach(func() {
				getCFServiceInstanceError = apierrors.NewNotFound(schema.GroupResource{}, cfServiceInstance.Name)
			})
			It("requeues the request", func() {
				Expect(reconcileResult).To(Equal(ctrl.Result{RequeueAfter: 2 * time.Second}))
				Expect(reconcileErr).NotTo(HaveOccurred())

				Expect(fakeStatusWriter.PatchCallCount()).To(Equal(1))
				_, serviceBindingObj, _, _ := fakeStatusWriter.PatchArgsForCall(0)
				updatedCFServiceBinding, ok := serviceBindingObj.(*korifiv1alpha1.CFServiceBinding)
				Expect(ok).To(BeTrue())
				Expect(updatedCFServiceBinding.Status.Binding.Name).To(BeEmpty())
				Expect(updatedCFServiceBinding.Status.Conditions).To(ContainElement(MatchFields(IgnoreExtras, Fields{
					"Type":    Equal("BindingSecretAvailable"),
					"Status":  Equal(metav1.ConditionFalse),
					"Reason":  Equal("ServiceInstanceNotFound"),
					"Message": Equal("Service instance does not exist"),
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
				_, serviceBindingObj, _, _ := fakeStatusWriter.PatchArgsForCall(0)
				updatedCFServiceBinding, ok := serviceBindingObj.(*korifiv1alpha1.CFServiceBinding)
				Expect(ok).To(BeTrue())
				Expect(updatedCFServiceBinding.Status.Binding.Name).To(BeEmpty())
				Expect(updatedCFServiceBinding.Status.Conditions).To(ContainElement(MatchFields(IgnoreExtras, Fields{
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
				_, serviceBindingObj, _, _ := fakeStatusWriter.PatchArgsForCall(0)
				updatedCFServiceBinding, ok := serviceBindingObj.(*korifiv1alpha1.CFServiceBinding)
				Expect(ok).To(BeTrue())
				Expect(updatedCFServiceBinding.Status.Binding.Name).To(BeEmpty())
				Expect(updatedCFServiceBinding.Status.Conditions).To(ContainElement(MatchFields(IgnoreExtras, Fields{
					"Type":    Equal("BindingSecretAvailable"),
					"Status":  Equal(metav1.ConditionFalse),
					"Reason":  Equal("UnknownError"),
					"Message": Equal("Error occurred while fetching binding secret: " + getCFServiceInstanceSecretError.Error()),
				})))
			})
		})
		When("the cfapp vcap services secret status is not set", func() {
			BeforeEach(func() {
				cfAppStatus = korifiv1alpha1.CFAppStatus{}
			})
			It("requeues the request", func() {
				Expect(reconcileResult).To(Equal(ctrl.Result{RequeueAfter: 2 * time.Second}))
				Expect(reconcileErr).NotTo(HaveOccurred())

				Expect(fakeStatusWriter.PatchCallCount()).To(Equal(1))
				_, serviceBindingObj, _, _ := fakeStatusWriter.PatchArgsForCall(0)
				updatedCFServiceBinding, ok := serviceBindingObj.(*korifiv1alpha1.CFServiceBinding)
				Expect(ok).To(BeTrue())
				Expect(updatedCFServiceBinding.Status.Binding.Name).To(BeEmpty())
				Expect(updatedCFServiceBinding.Status.Conditions).To(ContainElement(MatchFields(IgnoreExtras, Fields{
					"Type":   Equal("VCAPServicesSecretAvailable"),
					"Status": Equal(metav1.ConditionFalse),
				})))
			})
		})
		When("the vcap services secret isn't found", func() {
			BeforeEach(func() {
				cfAppStatus = korifiv1alpha1.CFAppStatus{
					VCAPServicesSecretName: "unfindable",
				}
			})

			It("requeues the request", func() {
				Expect(reconcileResult).To(Equal(ctrl.Result{RequeueAfter: 2 * time.Second}))
				Expect(reconcileErr).NotTo(HaveOccurred())
			})
			It("sets the VCAPServicesSecretAvailable condition to false", func() {
				Expect(fakeStatusWriter.PatchCallCount()).To(Equal(1))
				_, serviceBindingObj, _, _ := fakeStatusWriter.PatchArgsForCall(0)
				updatedCFServiceBinding, ok := serviceBindingObj.(*korifiv1alpha1.CFServiceBinding)
				Expect(ok).To(BeTrue())
				Expect(updatedCFServiceBinding.Status.Conditions).To(ContainElement(MatchFields(IgnoreExtras, Fields{
					"Type":   Equal("VCAPServicesSecretAvailable"),
					"Status": Equal(metav1.ConditionFalse),
					"Reason": Equal("SecretNotFound"),
				})))
			})
		})
		When("The API errors setting status on the CFServiceBinding", func() {
			BeforeEach(func() {
				updateCFServiceBindingStatusError = errors.New("another random error")
			})

			It("errors", func() {
				Expect(reconcileErr).To(HaveOccurred())
			})
		})
		When("building the vcap services env var value fails", func() {
			BeforeEach(func() {
				fakeBuilder.BuildVCAPServicesEnvValueReturns("", errors.New("build-env-vars-error"))
			})

			It("returns the error", func() {
				Expect(reconcileErr).To(MatchError("build-env-vars-error"))
			})
		})
		When("patching the vcap services secret fails", func() {
			BeforeEach(func() {
				patchSecretError = errors.New("secret-patch-error")
			})

			It("returns the error", func() {
				Expect(reconcileErr).To(MatchError("secret-patch-error"))
			})
		})
	})
})
