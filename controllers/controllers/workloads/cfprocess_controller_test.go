package workloads_test

import (
	"context"
	"errors"
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

var _ = Describe("CFProcessReconciler Unit Tests", func() {
	const (
		serviceSecretGUID = "service-secret-guid"
	)

	var (
		fakeClient *fake.Client

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

		cfBuildError            error
		cfAppError              error
		cfProcessError          error
		appEnvSecretError       error
		cfServiceSecretError    error
		cfServiceInstanceError  error
		lrpError                error
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

		cfApp = BuildCFAppCRObject(testAppGUID, testNamespace)
		appEnvSecretGUID = cfApp.Spec.EnvSecretName
		cfAppError = nil
		cfBuild = BuildCFBuildObject(testBuildGUID, testNamespace, testPackageGUID, testAppGUID)
		UpdateCFBuildWithDropletStatus(cfBuild)
		cfBuildError = nil
		cfProcess = BuildCFProcessCRObject(testProcessGUID, testNamespace, testAppGUID, testProcessType, testProcessCommand)
		cfProcessError = nil
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
			},
			Status: servicesv1alpha1.CFServiceBindingStatus{
				Binding: corev1.LocalObjectReference{Name: serviceSecretGUID},
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
		cfServiceInstanceError = nil
		lrp = nil
		lrpError = nil
		lrpListError = nil

		fakeClient.GetStub = func(_ context.Context, name types.NamespacedName, obj client.Object) error {
			// cast obj to find its kind
			switch obj := obj.(type) {
			case *workloadsv1alpha1.CFProcess:
				cfProcess.DeepCopyInto(obj)
				return cfProcessError
			case *workloadsv1alpha1.CFBuild:
				cfBuild.DeepCopyInto(obj)
				return cfBuildError
			case *workloadsv1alpha1.CFApp:
				cfApp.DeepCopyInto(obj)
				return cfAppError
			case *corev1.Secret:
				if name.Name == appEnvSecretGUID {
					appEnvSecret.DeepCopyInto(obj)
					return appEnvSecretError
				} else {
					cfServiceSecret.DeepCopyInto(obj)
					return cfServiceSecretError
				}
			case *servicesv1alpha1.CFServiceInstance:
				cfServiceInstance.DeepCopyInto(obj)
				return cfServiceInstanceError
			case *eiriniv1.LRP:
				if lrp != nil && lrpError == nil {
					lrp.DeepCopyInto(obj)
				}
				return lrpError
			default:
				panic("TestClient Get provided a weird obj")
			}
		}

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
					lrpError = apierrors.NewNotFound(schema.GroupResource{}, "some-guid")

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
					cfProcessError = errors.New(failsOnPurposeErrorMessage)
					_, reconcileErr = cfProcessReconciler.Reconcile(ctx, req)
				})

				It("returns an error", func() {
					Expect(reconcileErr).To(MatchError(failsOnPurposeErrorMessage))
				})
			})

			When("fetch CFProcess returns a NotFoundError", func() {
				BeforeEach(func() {
					cfProcessError = apierrors.NewNotFound(schema.GroupResource{}, cfProcess.Name)
					_, reconcileErr = cfProcessReconciler.Reconcile(ctx, req)
				})

				It("doesn't return an error", func() {
					Expect(reconcileErr).NotTo(HaveOccurred())
				})
			})

			When("fetch CFApp returns an error", func() {
				BeforeEach(func() {
					cfAppError = errors.New(failsOnPurposeErrorMessage)
					_, reconcileErr = cfProcessReconciler.Reconcile(ctx, req)
				})

				It("returns an error", func() {
					Expect(reconcileErr).To(MatchError(failsOnPurposeErrorMessage))
				})
			})

			When("fetch CFBuild returns an error", func() {
				BeforeEach(func() {
					cfBuildError = errors.New(failsOnPurposeErrorMessage)
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
					cfServiceInstanceError = errors.New(failsOnPurposeErrorMessage)
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
