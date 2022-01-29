package workloads_test

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	servicesv1alpha1 "code.cloudfoundry.org/cf-k8s-controllers/controllers/apis/services/v1alpha1"

	networkingv1alpha1 "code.cloudfoundry.org/cf-k8s-controllers/controllers/apis/networking/v1alpha1"

	eiriniv1 "code.cloudfoundry.org/eirini-controller/pkg/apis/eirini/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	workloadsv1alpha1 "code.cloudfoundry.org/cf-k8s-controllers/controllers/apis/workloads/v1alpha1"
	. "code.cloudfoundry.org/cf-k8s-controllers/controllers/controllers/workloads"
	"code.cloudfoundry.org/cf-k8s-controllers/controllers/controllers/workloads/fake"
	. "code.cloudfoundry.org/cf-k8s-controllers/controllers/controllers/workloads/testutils"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

const (
	testNamespace      = "test-ns"
	testProcessGUID    = "test-process-guid"
	testProcessType    = "web"
	testProcessCommand = "test-process-command"
	testAppGUID        = "test-app-guid"
	testBuildGUID      = "test-build-guid"
	testPackageGUID    = "test-package-guid"
)

type GetStub func(context.Context, types.NamespacedName, client.Object) error

type GetStubber struct {
	stub GetStub
}

func (s *GetStubber) Stub(ctx context.Context, name types.NamespacedName, object client.Object) error {
	return s.stub(ctx, name, object)
}

func (s *GetStubber) AddStub(stub func(GetStub) GetStub) {
	origStub := s.stub
	if origStub == nil {
		origStub = func(ctx context.Context, name types.NamespacedName, object client.Object) error {
			panic(fmt.Sprintf("GetStubber encountered an object without a stub to handle it: %T", object))
		}
	}
	s.stub = stub(origStub)
}

func ErrorForType[T client.Object](err error) func(GetStub) GetStub {
	return func(origStub GetStub) GetStub {
		return func(ctx context.Context, nsName types.NamespacedName, obj client.Object) error {
			if _, ok := obj.(T); ok {
				return err
			}
			return origStub(ctx, nsName, obj)
		}
	}
}

type DeepCopyIntoer[T any] interface {
	DeepCopyInto(T)
}

func AssignForType[D DeepCopyIntoer[D]](typeObj D) func(GetStub) GetStub {
	return func(origStub GetStub) GetStub {
		return func(ctx context.Context, nsName types.NamespacedName, obj client.Object) error {
			if o, ok := obj.(D); ok {
				typeObj.DeepCopyInto(o)

				return nil
			}
			return origStub(ctx, nsName, obj)
		}
	}
}

func StubForType[T client.Object](newStub func(context.Context, types.NamespacedName, T) error) func(GetStub) GetStub {
	return func(origStub GetStub) GetStub {
		return func(ctx context.Context, nsName types.NamespacedName, obj client.Object) error {
			if typedObj, ok := obj.(T); ok {
				return newStub(ctx, nsName, typedObj)
			}
			return origStub(ctx, nsName, obj)
		}
	}
}

var _ = Describe("CFProcessReconciler Unit Tests", func() {
	const (
		serviceSecretGUID = "service-secret-guid"
	)

	var (
		fakeClient *fake.Client
		getStubber *GetStubber

		cfBuild           *workloadsv1alpha1.CFBuild
		cfProcess         *workloadsv1alpha1.CFProcess
		cfApp             *workloadsv1alpha1.CFApp
		lrp               *eiriniv1.LRP
		routes            []networkingv1alpha1.CFRoute
		appEnvSecretGUID  string
		appEnvSecret      *corev1.Secret
		cfServiceSecret   *corev1.Secret
		cfServiceBinding  *servicesv1alpha1.CFServiceBinding
		cfServiceInstance *servicesv1alpha1.CFServiceInstance

		appEnvSecretError       error
		cfServiceSecretError    error
		lrpListError            error
		routeListError          error
		serviceBindingListError error

		cfProcessReconciler *CFProcessReconciler
		ctx                 context.Context
		req                 ctrl.Request

		reconcileErr error
	)

	BeforeEach(func() {
		fakeClient = new(fake.Client)
		getStubber = new(GetStubber)
		fakeClient.GetStub = getStubber.Stub

		cfApp = BuildCFAppCRObject(testAppGUID, testNamespace)
		appEnvSecretGUID = cfApp.Spec.EnvSecretName
		cfBuild = BuildCFBuildObject(testBuildGUID, testNamespace, testPackageGUID, testAppGUID)
		UpdateCFBuildWithDropletStatus(cfBuild)
		cfProcess = BuildCFProcessCRObject(testProcessGUID, testNamespace, testAppGUID, testProcessType, testProcessCommand)
		appEnvSecret = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      appEnvSecretGUID,
				Namespace: testNamespace,
			},
		}
		appEnvSecretError = nil

		cfServiceSecret = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      serviceSecretGUID,
				Namespace: testNamespace,
			},
		}
		cfServiceSecretError = nil

		cfServiceBinding = &servicesv1alpha1.CFServiceBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "service-binding-guid",
				Namespace: testNamespace,
			},
			Spec: servicesv1alpha1.CFServiceBindingSpec{
				Service: corev1.ObjectReference{
					Kind:       "ServiceInstance",
					Name:       "service-binding-guid",
					APIVersion: "services.cloudfoundry.org/v1alpha1",
				},
				SecretName: serviceSecretGUID,
			},
		}
		serviceBindingListError = nil
		cfServiceInstance = &servicesv1alpha1.CFServiceInstance{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "service-instance-guid",
				Namespace: testNamespace,
			},
			Spec: servicesv1alpha1.CFServiceInstanceSpec{
				Name:       "service-binding-name",
				SecretName: serviceSecretGUID,
				Type:       "user-provided",
				Tags:       []string{},
			},
		}
		lrp = nil
		lrpListError = nil

		getStubber.AddStub(AssignForType(cfProcess))
		getStubber.AddStub(AssignForType(cfBuild))
		getStubber.AddStub(AssignForType(cfApp))
		getStubber.AddStub(AssignForType(cfServiceInstance))
		getStubber.AddStub(StubForType(func(ctx context.Context, name types.NamespacedName, obj *corev1.Secret) error {
			if name.Name == appEnvSecretGUID {
				appEnvSecret.DeepCopyInto(obj)
				return appEnvSecretError
			} else {
				cfServiceSecret.DeepCopyInto(obj)
				return cfServiceSecretError
			}
		}))

		fakeClient.ListStub = func(ctx context.Context, list client.ObjectList, option ...client.ListOption) error {
			switch listObj := list.(type) {
			case *eiriniv1.LRPList:
				lrpList := eiriniv1.LRPList{Items: []eiriniv1.LRP{}}
				if lrp != nil {
					lrpList.Items = append(lrpList.Items, *lrp)
				}
				lrpList.DeepCopyInto(listObj)
				return lrpListError
			case *networkingv1alpha1.CFRouteList:
				routeList := networkingv1alpha1.CFRouteList{Items: routes}

				routeList.DeepCopyInto(listObj)
				return routeListError
			case *servicesv1alpha1.CFServiceBindingList:
				serviceBindingList := servicesv1alpha1.CFServiceBindingList{Items: []servicesv1alpha1.CFServiceBinding{*cfServiceBinding}}
				serviceBindingList.DeepCopyInto(listObj)
				return serviceBindingListError
			default:
				panic("TestClient Get provided a weird obj")
			}
		}

		// configure a CFProcessReconciler with the client
		Expect(workloadsv1alpha1.AddToScheme(scheme.Scheme)).To(Succeed())
		cfProcessReconciler = &CFProcessReconciler{
			Client: fakeClient,
			Scheme: scheme.Scheme,
			Log:    zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)),
		}
		ctx = context.Background()
		req = ctrl.Request{
			NamespacedName: types.NamespacedName{
				Namespace: testNamespace,
				Name:      testProcessGUID,
			},
		}
	})

	When("CFProcessReconciler.Reconcile is called", func() {
		When("on the happy path", func() {
			When("the CFApp is created with desired state stopped", func() {
				BeforeEach(func() {
					cfApp.Spec.DesiredState = workloadsv1alpha1.StoppedState
					_, reconcileErr = cfProcessReconciler.Reconcile(ctx, req)
					Expect(reconcileErr).ToNot(HaveOccurred())
				})

				It("does not attempt to create any new LRPs", func() {
					Expect(fakeClient.CreateCallCount()).To(Equal(0), "Client.Create call count mismatch")
				})
			})

			When("the CFApp is updated from desired state STARTED to STOPPED", func() {
				BeforeEach(func() {
					cfApp.Spec.DesiredState = workloadsv1alpha1.StoppedState
					lrp = &eiriniv1.LRP{
						ObjectMeta: metav1.ObjectMeta{
							Name:         testProcessGUID,
							GenerateName: "",
							Namespace:    testNamespace,
							Labels: map[string]string{
								workloadsv1alpha1.CFProcessGUIDLabelKey: testProcessGUID,
							},
						},
						Spec: eiriniv1.LRPSpec{
							GUID:        testProcessGUID,
							ProcessType: testProcessType,
							AppName:     cfApp.Spec.Name,
							AppGUID:     testAppGUID,
							Image:       "test-image-ref",
							Instances:   0,
							MemoryMB:    100,
							DiskMB:      100,
							CPUWeight:   0,
						},
						Status: eiriniv1.LRPStatus{
							Replicas: 0,
						},
					}
					getStubber.AddStub(AssignForType(lrp))

					_, reconcileErr = cfProcessReconciler.Reconcile(ctx, req)
					Expect(reconcileErr).ToNot(HaveOccurred())
				})

				It("deletes any existing LRPs for the CFApp", func() {
					Expect(fakeClient.DeleteCallCount()).To(Equal(1), "Client.Delete call count mismatch")
				})
			})

			When("the CFApp is started and there are existing routes matching", func() {
				const testPort = 1234

				BeforeEach(func() {
					cfApp.Spec.DesiredState = workloadsv1alpha1.StartedState
					getStubber.AddStub(ErrorForType[*eiriniv1.LRP](apierrors.NewNotFound(schema.GroupResource{}, "some-guid")))

					routes = []networkingv1alpha1.CFRoute{
						{
							ObjectMeta: metav1.ObjectMeta{
								CreationTimestamp: metav1.Time{
									Time: time.Now(),
								},
							},
							Status: networkingv1alpha1.CFRouteStatus{
								Destinations: []networkingv1alpha1.Destination{
									{
										GUID: "some-other-guid",
										Port: testPort + 1000,
										AppRef: corev1.LocalObjectReference{
											Name: testAppGUID,
										},
										ProcessType: testProcessType,
										Protocol:    "http1",
									},
								},
							},
						},
						{
							ObjectMeta: metav1.ObjectMeta{
								CreationTimestamp: metav1.Time{
									Time: time.Now().Add(-5 * time.Second),
								},
							},
							Status: networkingv1alpha1.CFRouteStatus{
								Destinations: []networkingv1alpha1.Destination{
									{
										GUID: "some-guid",
										Port: testPort,
										AppRef: corev1.LocalObjectReference{
											Name: testAppGUID,
										},
										ProcessType: testProcessType,
										Protocol:    "http1",
									},
								},
							},
						},
					}
				})

				It("chooses the oldest matching route", func() {
					_, reconcileErr = cfProcessReconciler.Reconcile(ctx, req)
					Expect(reconcileErr).ToNot(HaveOccurred())

					_, obj, _ := fakeClient.CreateArgsForCall(0)
					returnedLRP := obj.(*eiriniv1.LRP)
					Expect(returnedLRP.Spec.Env).To(HaveKeyWithValue("PORT", strconv.Itoa(testPort)))
					Expect(returnedLRP.Spec.Env).To(HaveKeyWithValue("VCAP_APP_PORT", strconv.Itoa(testPort)))
				})
			})
		})

		When("on the unhappy path", func() {
			BeforeEach(func() {
				cfApp.Spec.DesiredState = workloadsv1alpha1.StartedState
			})

			When("fetch CFProcess returns an error", func() {
				BeforeEach(func() {
					getStubber.AddStub(ErrorForType[*workloadsv1alpha1.CFProcess](errors.New(failsOnPurposeErrorMessage)))
					_, reconcileErr = cfProcessReconciler.Reconcile(ctx, req)
				})

				It("returns an error", func() {
					Expect(reconcileErr).To(MatchError(failsOnPurposeErrorMessage))
				})
			})

			When("fetch CFProcess returns a NotFoundError", func() {
				BeforeEach(func() {
					getStubber.AddStub(ErrorForType[*workloadsv1alpha1.CFProcess](apierrors.NewNotFound(schema.GroupResource{}, cfProcess.Name)))
					_, reconcileErr = cfProcessReconciler.Reconcile(ctx, req)
				})

				It("doesn't return an error", func() {
					Expect(reconcileErr).NotTo(HaveOccurred())
				})
			})

			When("fetch CFApp returns an error", func() {
				BeforeEach(func() {
					getStubber.AddStub(ErrorForType[*workloadsv1alpha1.CFApp](errors.New(failsOnPurposeErrorMessage)))
					_, reconcileErr = cfProcessReconciler.Reconcile(ctx, req)
				})

				It("returns an error", func() {
					Expect(reconcileErr).To(MatchError(failsOnPurposeErrorMessage))
				})
			})

			When("fetch CFBuild returns an error", func() {
				BeforeEach(func() {
					getStubber.AddStub(ErrorForType[*workloadsv1alpha1.CFBuild](errors.New(failsOnPurposeErrorMessage)))
					_, reconcileErr = cfProcessReconciler.Reconcile(ctx, req)
				})

				It("returns an error", func() {
					Expect(reconcileErr).To(MatchError(failsOnPurposeErrorMessage))
				})
			})

			When("CFBuild does not have a build droplet status", func() {
				BeforeEach(func() {
					cfBuild.Status.BuildDropletStatus = nil
					_, reconcileErr = cfProcessReconciler.Reconcile(ctx, req)
				})

				It("returns an error", func() {
					Expect(reconcileErr).To(MatchError("no build droplet status on CFBuild"))
				})
			})

			When("fetch Secret returns an error", func() {
				BeforeEach(func() {
					appEnvSecretError = errors.New(failsOnPurposeErrorMessage)
					_, reconcileErr = cfProcessReconciler.Reconcile(ctx, req)
				})

				It("returns an error", func() {
					Expect(reconcileErr).To(MatchError(failsOnPurposeErrorMessage))
				})
			})

			When("fetch CFServiceBinding list returns an error", func() {
				BeforeEach(func() {
					serviceBindingListError = errors.New(failsOnPurposeErrorMessage)
					_, reconcileErr = cfProcessReconciler.Reconcile(ctx, req)
				})

				It("returns an error", func() {
					Expect(reconcileErr).To(MatchError(ContainSubstring(failsOnPurposeErrorMessage)))
				})
			})

			When("fetch CFServiceInstance returns an error", func() {
				BeforeEach(func() {
					getStubber.AddStub(ErrorForType[*servicesv1alpha1.CFServiceInstance](errors.New(failsOnPurposeErrorMessage)))
					_, reconcileErr = cfProcessReconciler.Reconcile(ctx, req)
				})

				It("returns an error", func() {
					Expect(reconcileErr).To(MatchError(ContainSubstring(failsOnPurposeErrorMessage)))
				})
			})

			When("fetch CFServiceBinding Secret returns an error", func() {
				BeforeEach(func() {
					cfServiceSecretError = errors.New(failsOnPurposeErrorMessage)
					_, reconcileErr = cfProcessReconciler.Reconcile(ctx, req)
				})

				It("returns an error", func() {
					Expect(reconcileErr).To(MatchError(ContainSubstring(failsOnPurposeErrorMessage)))
				})
			})

			When("fetch LRPList returns an error", func() {
				BeforeEach(func() {
					getStubber.AddStub(StubForType(func(ctx context.Context, name types.NamespacedName, obj *eiriniv1.LRP) error {
						// Don't make any change to LRP to avoid a merge error
						return nil
					}))
					lrpListError = errors.New(failsOnPurposeErrorMessage)
					_, reconcileErr = cfProcessReconciler.Reconcile(ctx, req)
				})

				It("returns an error", func() {
					Expect(reconcileErr).To(MatchError(failsOnPurposeErrorMessage))
				})
			})
		})
	})
})
