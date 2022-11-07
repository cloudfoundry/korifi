package workloads_test

import (
	"context"
	"errors"
	"time"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	. "code.cloudfoundry.org/korifi/controllers/controllers/workloads"
	"code.cloudfoundry.org/korifi/controllers/controllers/workloads/fake"
	. "code.cloudfoundry.org/korifi/controllers/controllers/workloads/testutils"
	"code.cloudfoundry.org/korifi/tools/k8s"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

const (
	defaultNamespace               = "default"
	failsOnPurposeErrorMessage     = "fails on purpose"
	buildDropletStatusErrorMessage = "status field CFBuildDropletStatus is nil on CFBuild"
	labelSyntaxErrorMessage        = "a valid label must be an empty string or consist of alphanumeric characters"
)

var _ = Describe("CFAppReconciler", func() {
	var (
		cfAppGUID     string
		cfBuildGUID   string
		cfPackageGUID string

		cfBuild       *korifiv1alpha1.CFBuild
		cfBuildErr    error
		cfApp         *korifiv1alpha1.CFApp
		cfAppError    error
		cfAppPatchErr error

		cfRoutePatchErr  error
		cfRouteListErr   error
		cfProcessListErr error
		cfTaskListErr    error
		cfTaskDeleteErr  error

		secret         *corev1.Secret
		secretErr      error
		secretPatchErr error

		cfAppReconciler *k8s.PatchingReconciler[korifiv1alpha1.CFApp, *korifiv1alpha1.CFApp]
		ctx             context.Context
		req             ctrl.Request

		fakeBuilder *fake.VCAPServicesSecretBuilder

		reconcileResult ctrl.Result
		reconcileErr    error
	)

	BeforeEach(func() {
		cfAppGUID = "cf-app-guid"
		cfPackageGUID = "cf-package-guid"
		cfBuildGUID = "cf-build-guid"

		cfApp = BuildCFAppCRObject(cfAppGUID, defaultNamespace)
		cfAppError = nil
		cfAppPatchErr = nil
		cfBuild = BuildCFBuildObject(cfBuildGUID, defaultNamespace, cfPackageGUID, cfAppGUID)
		UpdateCFBuildWithDropletStatus(cfBuild)
		cfBuildErr = nil

		cfRoutePatchErr = nil
		cfRouteListErr = nil

		secret = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: cfApp.Namespace,
			},
		}
		secretErr = nil
		secretPatchErr = nil

		fakeClient.GetStub = func(_ context.Context, _ types.NamespacedName, obj client.Object, _ ...client.GetOption) error {
			// cast obj to find its kind
			switch obj := obj.(type) {
			case *korifiv1alpha1.CFBuild:
				cfBuild.DeepCopyInto(obj)
				return cfBuildErr
			case *korifiv1alpha1.CFApp:
				cfApp.DeepCopyInto(obj)
				return cfAppError
			case *corev1.Secret:
				originalName := obj.GetName()
				secret.DeepCopyInto(obj)
				obj.Name = originalName
				return secretErr
			default:
				panic("TestClient Get provided a weird obj")
			}
		}

		cfProcessList := korifiv1alpha1.CFProcessList{}
		cfProcessListErr = nil
		cfRouteList := korifiv1alpha1.CFRouteList{
			Items: []korifiv1alpha1.CFRoute{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "cfRouteGUID",
						Namespace: defaultNamespace,
					},
					Spec: korifiv1alpha1.CFRouteSpec{
						Host:     "testRouteHost",
						Path:     "",
						Protocol: "http",
						DomainRef: corev1.ObjectReference{
							Name:      "testDomainGUID",
							Namespace: defaultNamespace,
						},
						Destinations: []korifiv1alpha1.Destination{
							{
								GUID: "destination-1-guid",
								Port: 0,
								AppRef: corev1.LocalObjectReference{
									Name: cfAppGUID,
								},
								ProcessType: "web",
								Protocol:    "http1",
							},
							{
								GUID: "destination-2-guid",
								Port: 0,
								AppRef: corev1.LocalObjectReference{
									Name: "some-other-app-guid",
								},
								ProcessType: "worked",
								Protocol:    "http1",
							},
						},
					},
				},
			},
		}

		cfTaskListErr = nil
		cfTaskList := korifiv1alpha1.CFTaskList{
			Items: []korifiv1alpha1.CFTask{{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cfTaskGUID",
					Namespace: defaultNamespace,
				},
			}},
		}
		fakeClient.ListStub = func(ctx context.Context, list client.ObjectList, option ...client.ListOption) error {
			switch list := list.(type) {
			case *korifiv1alpha1.CFProcessList:
				cfProcessList.DeepCopyInto(list)
				return cfProcessListErr
			case *korifiv1alpha1.CFRouteList:
				cfRouteList.DeepCopyInto(list)
				return cfRouteListErr
			case *korifiv1alpha1.CFTaskList:
				cfTaskList.DeepCopyInto(list)
				return cfTaskListErr
			default:
				panic("TestClient List provided a weird obj")
			}
		}

		fakeClient.PatchStub = func(ctx context.Context, object client.Object, patch client.Patch, option ...client.PatchOption) error {
			switch object.(type) {
			case *korifiv1alpha1.CFRoute:
				return cfRoutePatchErr
			case *korifiv1alpha1.CFApp:
				return cfAppPatchErr
			case *corev1.Secret:
				return secretPatchErr
			default:
				panic("TestClient Patch provided an unexpected object type")
			}
		}

		cfTaskDeleteErr = nil
		fakeClient.DeleteStub = func(ctx context.Context, object client.Object, option ...client.DeleteOption) error {
			switch object.(type) {
			case *korifiv1alpha1.CFTask:
				return cfTaskDeleteErr
			default:
				panic("TestClient Delete provided an unexpected object type")
			}
		}

		fakeBuilder = new(fake.VCAPServicesSecretBuilder)

		// configure a CFAppReconciler with the client
		Expect(korifiv1alpha1.AddToScheme(scheme.Scheme)).To(Succeed())
		cfAppReconciler = NewCFAppReconciler(
			fakeClient,
			scheme.Scheme,
			zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)),
			fakeBuilder,
		)
		ctx = context.Background()
		req = ctrl.Request{
			NamespacedName: types.NamespacedName{
				Namespace: defaultNamespace,
				Name:      cfAppGUID,
			},
		}
	})

	JustBeforeEach(func() {
		reconcileResult, reconcileErr = cfAppReconciler.Reconcile(ctx, req)
	})

	When("a CFApp is created and CFAppReconciler Reconcile function is called", func() {
		When("on the happy path", func() {
			It("returns an empty result and does not return error", func() {
				Expect(reconcileResult).To(Equal(ctrl.Result{}))
				Expect(reconcileErr).NotTo(HaveOccurred())

				// validate the inputs to Get
				Expect(fakeClient.GetCallCount()).To(BeNumerically(">=", 1))
				_, testRequestNamespacedName, _, _ := fakeClient.GetArgsForCall(0)
				Expect(testRequestNamespacedName.Namespace).To(Equal(defaultNamespace))
				Expect(testRequestNamespacedName.Name).To(Equal(cfAppGUID))

				Expect(fakeStatusWriter.PatchCallCount()).To(Equal(1))
				_, updatedCFApp, _, _ := fakeStatusWriter.PatchArgsForCall(0)
				cast, ok := updatedCFApp.(*korifiv1alpha1.CFApp)
				Expect(ok).To(BeTrue(), "Cast to korifiv1alpha1.CFApp failed")
				Expect(meta.IsStatusConditionTrue(cast.Status.Conditions, StatusConditionStaged)).To(BeFalse())
				Expect(meta.IsStatusConditionFalse(cast.Status.Conditions, StatusConditionRunning)).To(BeTrue(), "Status Condition "+StatusConditionRunning+" was not False as expected")
				Expect(cast.Status.ObservedDesiredState).To(Equal(cast.Spec.DesiredState))
			})
		})

		When("on the unhappy path", func() {
			When("fetch CFApp returns an error", func() {
				BeforeEach(func() {
					cfAppError = errors.New(failsOnPurposeErrorMessage)
				})

				It("should return an error", func() {
					Expect(reconcileErr).To(MatchError(failsOnPurposeErrorMessage))
				})
			})

			When("fetch CFApp returns a NotFoundError", func() {
				BeforeEach(func() {
					cfAppError = apierrors.NewNotFound(schema.GroupResource{}, cfBuild.Name)
				})

				It("should NOT return an error", func() {
					Expect(reconcileErr).NotTo(HaveOccurred())
				})
			})

			When("patch status conditions returns an error", func() {
				BeforeEach(func() {
					fakeStatusWriter.PatchReturns(errors.New(failsOnPurposeErrorMessage))
				})

				It("should returns an error", func() {
					Expect(reconcileErr).To(MatchError(failsOnPurposeErrorMessage))
				})
			})

			When("fetch vcap services Secret returns an error", func() {
				BeforeEach(func() {
					secretErr = errors.New(failsOnPurposeErrorMessage)
				})

				It("should return an error", func() {
					Expect(reconcileErr).To(MatchError(failsOnPurposeErrorMessage))
				})
			})

			When("fetch vcap services Secret returns a NotFoundError and create Secret returns an error", func() {
				BeforeEach(func() {
					secretErr = apierrors.NewNotFound(schema.GroupResource{}, cfBuild.Name)
					fakeClient.CreateReturns(errors.New(failsOnPurposeErrorMessage))
				})

				It("should return an error", func() {
					Expect(reconcileErr).To(MatchError(failsOnPurposeErrorMessage))
				})
			})

			When("building the VCAP_SERVICES value fails", func() {
				BeforeEach(func() {
					fakeBuilder.BuildVCAPServicesEnvValueReturns("", errors.New("vcap-services-build-err"))
				})

				It("should return an error", func() {
					Expect(reconcileErr).To(MatchError(ContainSubstring("vcap-services-build-err")))
				})
			})
		})
	})

	When("a CFApp is updated to set currentDropletRef which has a web process and Reconcile function is called", func() {
		BeforeEach(func() {
			cfApp.Spec.CurrentDropletRef = corev1.LocalObjectReference{Name: cfBuildGUID}
		})

		When("on the happy path", func() {
			It("returns an empty result and does not return error", func() {
				Expect(reconcileResult).To(Equal(ctrl.Result{}))
				Expect(reconcileErr).NotTo(HaveOccurred())

				// validate the inputs to Get
				Expect(fakeClient.GetCallCount()).To(Equal(3))

				// Validate args to fetch CFApp
				_, testRequestNamespacedName, _, _ := fakeClient.GetArgsForCall(0)
				Expect(testRequestNamespacedName.Namespace).To(Equal(defaultNamespace))
				Expect(testRequestNamespacedName.Name).To(Equal(cfAppGUID))

				// Validate args to fetch CFBuild
				_, testRequestNamespacedName, _, _ = fakeClient.GetArgsForCall(2)
				Expect(testRequestNamespacedName.Namespace).To(Equal(defaultNamespace))
				Expect(testRequestNamespacedName.Name).To(Equal(cfBuildGUID))

				// Validate call count to fetch CFProcess
				Expect(fakeClient.ListCallCount()).To(Equal(1))

				// Validate call count to create CFProcess
				Expect(fakeClient.CreateCallCount()).To(Equal(1))
				_, desiredProcess, _ := fakeClient.CreateArgsForCall(0)
				Expect(desiredProcess.GetName()).To(Equal("cf-proc-cf-app-guid-web"))

				Expect(fakeStatusWriter.PatchCallCount()).To(Equal(1))
				_, updatedCFApp, _, _ := fakeStatusWriter.PatchArgsForCall(0)
				cast, ok := updatedCFApp.(*korifiv1alpha1.CFApp)
				Expect(ok).To(BeTrue(), "Cast to korifiv1alpha1.CFApp failed")
				Expect(meta.IsStatusConditionTrue(cast.Status.Conditions, StatusConditionStaged)).To(BeTrue())
				Expect(meta.IsStatusConditionFalse(cast.Status.Conditions, StatusConditionRunning)).To(BeTrue(), "Status Condition "+StatusConditionRunning+" was not False as expected")
			})
		})

		When("on the unhappy path", func() {
			When("fetch CFApp returns an error", func() {
				BeforeEach(func() {
					cfAppError = errors.New(failsOnPurposeErrorMessage)
				})

				It("should returns an error", func() {
					Expect(reconcileErr).To(MatchError(failsOnPurposeErrorMessage))
				})
			})

			When("fetch CFApp returns a NotFoundError", func() {
				BeforeEach(func() {
					cfAppError = apierrors.NewNotFound(schema.GroupResource{}, cfBuild.Name)
				})

				It("should NOT return an error", func() {
					Expect(reconcileErr).NotTo(HaveOccurred())
				})
			})

			When("fetch CFBuild returns an error", func() {
				BeforeEach(func() {
					cfBuildErr = errors.New(failsOnPurposeErrorMessage)
				})

				It("should returns an error", func() {
					Expect(reconcileErr).To(MatchError(failsOnPurposeErrorMessage))
				})

				It("should unset the staged condition", func() {
					Expect(fakeStatusWriter.PatchCallCount()).To(Equal(1))
					_, updatedCFApp, _, _ := fakeStatusWriter.PatchArgsForCall(0)
					cast, ok := updatedCFApp.(*korifiv1alpha1.CFApp)
					Expect(ok).To(BeTrue(), "Cast to korifiv1alpha1.CFApp failed")
					Expect(meta.IsStatusConditionTrue(cast.Status.Conditions, StatusConditionStaged)).To(BeFalse())
				})
			})

			When("Droplet status on CFBuild is nil", func() {
				BeforeEach(func() {
					cfBuild.Status.Droplet = nil
				})

				It("should returns an error", func() {
					Expect(reconcileErr).To(MatchError(ContainSubstring(buildDropletStatusErrorMessage)))
				})

				It("should unset the staged condition", func() {
					Expect(fakeStatusWriter.PatchCallCount()).To(Equal(1))
					_, updatedCFApp, _, _ := fakeStatusWriter.PatchArgsForCall(0)
					cast, ok := updatedCFApp.(*korifiv1alpha1.CFApp)
					Expect(ok).To(BeTrue(), "Cast to korifiv1alpha1.CFApp failed")
					Expect(meta.IsStatusConditionTrue(cast.Status.Conditions, StatusConditionStaged)).To(BeFalse())
				})
			})

			When("Label value doesnt conform to the syntax", func() {
				BeforeEach(func() {
					cfBuild.Status.Droplet.ProcessTypes[0].Type = "#web"
				})

				It("should returns an error", func() {
					Expect(reconcileErr).To(MatchError(ContainSubstring(labelSyntaxErrorMessage)))
				})
			})

			When("fetch matching CFProcess returns error", func() {
				BeforeEach(func() {
					cfProcessListErr = errors.New(failsOnPurposeErrorMessage)
				})

				It("should returns an error", func() {
					Expect(reconcileErr).To(MatchError(failsOnPurposeErrorMessage))
				})

				It("has a staged true condition", func() {
					Expect(fakeStatusWriter.PatchCallCount()).To(Equal(1))
					_, updatedCFApp, _, _ := fakeStatusWriter.PatchArgsForCall(0)
					cast, ok := updatedCFApp.(*korifiv1alpha1.CFApp)
					Expect(ok).To(BeTrue())
					Expect(meta.IsStatusConditionTrue(cast.Status.Conditions, StatusConditionStaged)).To(BeTrue())
				})
			})

			When("create CFProcess returns an error", func() {
				BeforeEach(func() {
					fakeClient.CreateReturns(errors.New(failsOnPurposeErrorMessage))
				})

				It("should returns an error", func() {
					Expect(reconcileErr).To(MatchError(failsOnPurposeErrorMessage))
				})

				It("has a staged true condition", func() {
					Expect(fakeStatusWriter.PatchCallCount()).To(Equal(1))
					_, updatedCFApp, _, _ := fakeStatusWriter.PatchArgsForCall(0)
					cast, ok := updatedCFApp.(*korifiv1alpha1.CFApp)
					Expect(ok).To(BeTrue())
					Expect(meta.IsStatusConditionTrue(cast.Status.Conditions, StatusConditionStaged)).To(BeTrue())
				})
			})

			When("patch status conditions returns an error", func() {
				BeforeEach(func() {
					fakeStatusWriter.PatchReturns(errors.New(failsOnPurposeErrorMessage))
				})

				It("should returns an error", func() {
					Expect(reconcileErr).To(MatchError(failsOnPurposeErrorMessage))
				})
			})

			When("patch status returns an error", func() {
				BeforeEach(func() {
					fakeStatusWriter.PatchReturns(errors.New(failsOnPurposeErrorMessage))
				})

				It("should returns an error", func() {
					Expect(reconcileErr).To(MatchError(failsOnPurposeErrorMessage))
				})
			})

			When("adding the finalizer to the CFApp returns an error", func() {
				BeforeEach(func() {
					cfApp.Finalizers = []string{}
					cfAppPatchErr = errors.New("failed to patch CFApp")
				})

				It("return the error", func() {
					Expect(reconcileErr).To(MatchError("failed to patch CFApp"))
				})
			})
		})
	})

	When("a CFApp is being deleted", func() {
		BeforeEach(func() {
			cfApp.DeletionTimestamp = &metav1.Time{
				Time: time.Now(),
			}
		})

		When("on the happy path", func() {
			It("returns an empty result and does not return error", func() {
				Expect(reconcileResult).To(Equal(ctrl.Result{}))
				Expect(reconcileErr).NotTo(HaveOccurred())
			})

			It("removes the finalizer from the CFApp", func() {
				_, requestObject, _, _ := fakeClient.PatchArgsForCall(1)
				requestApp, ok := requestObject.(*korifiv1alpha1.CFApp)
				Expect(ok).To(BeTrue(), "Cast to korifiv1alpha1.CFApp failed")
				Expect(requestApp.ObjectMeta.Finalizers).To(HaveLen(0), "CFApp finalizer count mismatch")
			})

			It("does not attempt to create any resources", func() {
				Expect(fakeClient.CreateCallCount()).To(Equal(0), "Client.Create call count mismatch")
			})
		})

		When("on the unhappy path", func() {
			When("fetching CFRoutes return an error", func() {
				BeforeEach(func() {
					cfRouteListErr = errors.New("failed to list CFRoute")
				})

				It("return the error", func() {
					Expect(reconcileErr).To(MatchError("failed to list CFRoute"))
				})
			})

			When("patching cfRoute returns an error", func() {
				BeforeEach(func() {
					cfRoutePatchErr = errors.New("failed to patch CFRoute")
				})

				It("return the error", func() {
					Expect(reconcileErr).To(MatchError("failed to patch CFRoute"))
				})
			})

			When("listing the app tasks fails", func() {
				BeforeEach(func() {
					cfTaskListErr = errors.New("failed to list CFTask")
				})

				It("return the error", func() {
					Expect(reconcileErr).To(MatchError("failed to list CFTask"))
				})
			})

			When("deleting an app task fails", func() {
				BeforeEach(func() {
					cfTaskDeleteErr = errors.New("failed to delete CFTask")
				})

				It("return the error", func() {
					Expect(reconcileErr).To(MatchError("failed to delete CFTask"))
				})
			})

			When("patching cfApp returns an error", func() {
				BeforeEach(func() {
					cfAppPatchErr = errors.New("failed to patch CFApp")
				})

				It("return the error", func() {
					Expect(reconcileErr).To(MatchError("failed to patch CFApp"))
				})
			})
		})
	})
})
