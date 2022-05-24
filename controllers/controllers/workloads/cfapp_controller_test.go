package workloads_test

import (
	"context"
	"errors"
	"time"

	"code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/config"
	. "code.cloudfoundry.org/korifi/controllers/controllers/workloads"
	workloadsfakes "code.cloudfoundry.org/korifi/controllers/controllers/workloads/fake"
	. "code.cloudfoundry.org/korifi/controllers/controllers/workloads/testutils"
	"code.cloudfoundry.org/korifi/controllers/fake"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
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
		fakeClient       *workloadsfakes.CFClient
		fakeStatusWriter *fake.StatusWriter

		cfAppGUID     string
		cfBuildGUID   string
		cfPackageGUID string

		cfBuild       *v1alpha1.CFBuild
		cfBuildError  error
		cfApp         *v1alpha1.CFApp
		cfAppError    error
		cfAppPatchErr error

		cfRoutePatchErr error
		cfRouteListErr  error

		cfAppReconciler *CFAppReconciler
		ctx             context.Context
		req             ctrl.Request

		reconcileResult ctrl.Result
		reconcileErr    error
	)

	BeforeEach(func() {
		fakeClient = new(workloadsfakes.CFClient)

		cfAppGUID = "cf-app-guid"
		cfPackageGUID = "cf-package-guid"
		cfBuildGUID = "cf-build-guid"

		cfApp = BuildCFAppCRObject(cfAppGUID, defaultNamespace)
		cfAppError = nil
		cfAppPatchErr = nil
		cfBuild = BuildCFBuildObject(cfBuildGUID, defaultNamespace, cfPackageGUID, cfAppGUID)
		UpdateCFBuildWithDropletStatus(cfBuild)
		cfBuildError = nil

		cfRoutePatchErr = nil
		cfRouteListErr = nil

		fakeClient.GetStub = func(_ context.Context, _ types.NamespacedName, obj client.Object) error {
			// cast obj to find its kind
			switch obj := obj.(type) {
			case *v1alpha1.CFBuild:
				cfBuild.DeepCopyInto(obj)
				return cfBuildError
			case *v1alpha1.CFApp:
				cfApp.DeepCopyInto(obj)
				return cfAppError
			default:
				panic("TestClient Get provided a weird obj")
			}
		}

		// Configure mock status update to succeed
		fakeStatusWriter = &fake.StatusWriter{}
		fakeClient.StatusReturns(fakeStatusWriter)

		cfProcessList := v1alpha1.CFProcessList{}
		cfRouteList := v1alpha1.CFRouteList{
			Items: []v1alpha1.CFRoute{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "cfRouteGUID",
						Namespace: defaultNamespace,
					},
					Spec: v1alpha1.CFRouteSpec{
						Host:     "testRouteHost",
						Path:     "",
						Protocol: "http",
						DomainRef: v1.ObjectReference{
							Name:      "testDomainGUID",
							Namespace: defaultNamespace,
						},
						Destinations: []v1alpha1.Destination{
							{
								GUID: "destination-1-guid",
								Port: 0,
								AppRef: v1.LocalObjectReference{
									Name: cfAppGUID,
								},
								ProcessType: "web",
								Protocol:    "http1",
							},
							{
								GUID: "destination-2-guid",
								Port: 0,
								AppRef: v1.LocalObjectReference{
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

		fakeClient.ListStub = func(ctx context.Context, list client.ObjectList, option ...client.ListOption) error {
			switch list := list.(type) {
			case *v1alpha1.CFProcessList:
				cfProcessList.DeepCopyInto(list)
				return nil
			case *v1alpha1.CFRouteList:
				cfRouteList.DeepCopyInto(list)
				return cfRouteListErr
			default:
				panic("TestClient List provided a weird obj")
			}
		}

		fakeClient.PatchStub = func(ctx context.Context, object client.Object, patch client.Patch, option ...client.PatchOption) error {
			switch object.(type) {
			case *v1alpha1.CFRoute:
				return cfRoutePatchErr
			case *v1alpha1.CFApp:
				return cfAppPatchErr
			default:
				panic("TestClient Patch provided an unexpected object type")
			}
		}

		// configure a CFAppReconciler with the client
		Expect(v1alpha1.AddToScheme(scheme.Scheme)).To(Succeed())
		cfAppReconciler = &CFAppReconciler{
			Client: fakeClient,
			Scheme: scheme.Scheme,
			Log:    zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)),
			ControllerConfig: &config.ControllerConfig{
				CFProcessDefaults: config.CFProcessDefaults{
					MemoryMB:           0,
					DefaultDiskQuotaMB: 0,
				},
			},
		}
		ctx = context.Background()
		req = ctrl.Request{
			NamespacedName: types.NamespacedName{
				Namespace: defaultNamespace,
				Name:      cfAppGUID,
			},
		}
	})

	When("a CFApp is created and CFAppReconciler Reconcile function is called", func() {
		When("on the happy path", func() {
			BeforeEach(func() {
				reconcileResult, reconcileErr = cfAppReconciler.Reconcile(ctx, req)
			})

			It("returns an empty result and does not return error", func() {
				Expect(reconcileResult).To(Equal(ctrl.Result{}))
				Expect(reconcileErr).NotTo(HaveOccurred())

				// validate the inputs to Get
				Expect(fakeClient.GetCallCount()).To(Equal(1))
				_, testRequestNamespacedName, _ := fakeClient.GetArgsForCall(0)
				Expect(testRequestNamespacedName.Namespace).To(Equal(defaultNamespace))
				Expect(testRequestNamespacedName.Name).To(Equal(cfAppGUID))

				// validate the inputs to Status.Update
				Expect(fakeStatusWriter.UpdateCallCount()).To(Equal(1))
				_, updatedCFApp, _ := fakeStatusWriter.UpdateArgsForCall(0)
				cast, ok := updatedCFApp.(*v1alpha1.CFApp)
				Expect(ok).To(BeTrue(), "Cast to v1alpha1.CFApp failed")
				Expect(meta.IsStatusConditionFalse(cast.Status.Conditions, StatusConditionRunning)).To(BeTrue(), "Status Condition "+StatusConditionRunning+" was not False as expected")
				Expect(cast.Status.ObservedDesiredState).To(Equal(cast.Spec.DesiredState))
			})
		})

		When("on the unhappy path", func() {
			When("fetch CFApp returns an error", func() {
				BeforeEach(func() {
					cfAppError = errors.New(failsOnPurposeErrorMessage)
					_, reconcileErr = cfAppReconciler.Reconcile(ctx, req)
				})

				It("should return an error", func() {
					Expect(reconcileErr).To(MatchError(failsOnPurposeErrorMessage))
				})
			})

			When("fetch CFApp returns a NotFoundError", func() {
				BeforeEach(func() {
					cfAppError = apierrors.NewNotFound(schema.GroupResource{}, cfBuild.Name)
					_, reconcileErr = cfAppReconciler.Reconcile(ctx, req)
				})

				It("should NOT return an error", func() {
					Expect(reconcileErr).NotTo(HaveOccurred())
				})
			})

			When("update status conditions returns an error", func() {
				BeforeEach(func() {
					fakeStatusWriter.UpdateReturns(errors.New(failsOnPurposeErrorMessage))
					_, reconcileErr = cfAppReconciler.Reconcile(ctx, req)
				})

				It("should returns an error", func() {
					Expect(reconcileErr).To(MatchError(failsOnPurposeErrorMessage))
				})
			})
		})
	})

	When("a CFApp is updated to set currentDropletRef and Reconcile function is called", func() {
		BeforeEach(func() {
			cfApp.Spec.CurrentDropletRef = v1.LocalObjectReference{Name: cfBuildGUID}
		})

		When("on the happy path", func() {
			BeforeEach(func() {
				reconcileResult, reconcileErr = cfAppReconciler.Reconcile(ctx, req)
			})

			It("returns an empty result and does not return error", func() {
				Expect(reconcileResult).To(Equal(ctrl.Result{}))
				Expect(reconcileErr).NotTo(HaveOccurred())

				// validate the inputs to Get
				Expect(fakeClient.GetCallCount()).To(Equal(2))

				// Validate args to fetch CFApp
				_, testRequestNamespacedName, _ := fakeClient.GetArgsForCall(0)
				Expect(testRequestNamespacedName.Namespace).To(Equal(defaultNamespace))
				Expect(testRequestNamespacedName.Name).To(Equal(cfAppGUID))

				// Validate args to fetch CFBuild
				_, testRequestNamespacedName, _ = fakeClient.GetArgsForCall(1)
				Expect(testRequestNamespacedName.Namespace).To(Equal(defaultNamespace))
				Expect(testRequestNamespacedName.Name).To(Equal(cfBuildGUID))

				// Validate call count to fetch CFProcess
				Expect(fakeClient.ListCallCount()).To(Equal(1))

				// Validate call count to create CFProcess
				Expect(fakeClient.CreateCallCount()).To(Equal(1))
				_, desiredProcess, _ := fakeClient.CreateArgsForCall(0)
				Expect(desiredProcess.GetName()).To(HavePrefix("cf-proc-"))

				// validate the inputs to Status.Update
				Expect(fakeStatusWriter.UpdateCallCount()).To(Equal(1))
				_, updatedCFApp, _ := fakeStatusWriter.UpdateArgsForCall(0)
				cast, ok := updatedCFApp.(*v1alpha1.CFApp)
				Expect(ok).To(BeTrue(), "Cast to v1alpha1.CFApp failed")
				Expect(meta.IsStatusConditionFalse(cast.Status.Conditions, StatusConditionRunning)).To(BeTrue(), "Status Condition "+StatusConditionRunning+" was not False as expected")
			})
		})

		When("on the unhappy path", func() {
			When("fetch CFApp returns an error", func() {
				BeforeEach(func() {
					cfAppError = errors.New(failsOnPurposeErrorMessage)
					_, reconcileErr = cfAppReconciler.Reconcile(ctx, req)
				})

				It("should returns an error", func() {
					Expect(reconcileErr).To(MatchError(failsOnPurposeErrorMessage))
				})
			})

			When("fetch CFApp returns a NotFoundError", func() {
				BeforeEach(func() {
					cfAppError = apierrors.NewNotFound(schema.GroupResource{}, cfBuild.Name)
					_, reconcileErr = cfAppReconciler.Reconcile(ctx, req)
				})

				It("should NOT return an error", func() {
					Expect(reconcileErr).NotTo(HaveOccurred())
				})
			})

			When("fetch CFBuild returns an error", func() {
				BeforeEach(func() {
					cfBuildError = errors.New(failsOnPurposeErrorMessage)
					_, reconcileErr = cfAppReconciler.Reconcile(ctx, req)
				})

				It("should returns an error", func() {
					Expect(reconcileErr).To(MatchError(failsOnPurposeErrorMessage))
				})
			})

			When("Droplet status on CFBuild is nil", func() {
				BeforeEach(func() {
					cfBuild.Status.Droplet = nil
					_, reconcileErr = cfAppReconciler.Reconcile(ctx, req)
				})

				It("should returns an error", func() {
					Expect(reconcileErr).To(MatchError(ContainSubstring(buildDropletStatusErrorMessage)))
				})
			})

			When("Label value doesnt conform to the syntax", func() {
				BeforeEach(func() {
					cfBuild.Status.Droplet.ProcessTypes[0].Type = "#web"
					_, reconcileErr = cfAppReconciler.Reconcile(ctx, req)
				})

				It("should returns an error", func() {
					Expect(reconcileErr).To(MatchError(ContainSubstring(labelSyntaxErrorMessage)))
				})
			})

			When("fetch matching CFProcess returns error", func() {
				BeforeEach(func() {
					fakeClient.ListReturns(errors.New(failsOnPurposeErrorMessage))
					_, reconcileErr = cfAppReconciler.Reconcile(ctx, req)
				})

				It("should returns an error", func() {
					Expect(reconcileErr).To(MatchError(failsOnPurposeErrorMessage))
				})
			})

			When("create CFProcess returns an error", func() {
				BeforeEach(func() {
					fakeClient.CreateReturns(errors.New(failsOnPurposeErrorMessage))
					_, reconcileErr = cfAppReconciler.Reconcile(ctx, req)
				})

				It("should returns an error", func() {
					Expect(reconcileErr).To(MatchError(failsOnPurposeErrorMessage))
				})
			})

			When("update status conditions returns an error", func() {
				BeforeEach(func() {
					fakeStatusWriter.UpdateReturns(errors.New(failsOnPurposeErrorMessage))
					_, reconcileErr = cfAppReconciler.Reconcile(ctx, req)
				})

				It("should returns an error", func() {
					Expect(reconcileErr).To(MatchError(failsOnPurposeErrorMessage))
				})
			})

			When("adding the finalizer to the CFApp returns an error", func() {
				BeforeEach(func() {
					cfApp.ObjectMeta.Finalizers = []string{}
					cfAppPatchErr = errors.New("failed to patch CFApp")
					_, reconcileErr = cfAppReconciler.Reconcile(ctx, req)
				})

				It("return the error", func() {
					Expect(reconcileErr).To(MatchError("failed to patch CFApp"))
				})
			})
		})
	})

	When("a CFApp is being deleted", func() {
		BeforeEach(func() {
			cfApp.ObjectMeta.DeletionTimestamp = &metav1.Time{
				Time: time.Now(),
			}
		})

		When("on the happy path", func() {
			BeforeEach(func() {
				reconcileResult, reconcileErr = cfAppReconciler.Reconcile(ctx, req)
			})

			It("returns an empty result and does not return error", func() {
				Expect(reconcileResult).To(Equal(ctrl.Result{}))
				Expect(reconcileErr).NotTo(HaveOccurred())
			})

			It("removes the finalizer from the CFApp", func() {
				_, requestObject, _, _ := fakeClient.PatchArgsForCall(1)
				requestApp, ok := requestObject.(*v1alpha1.CFApp)
				Expect(ok).To(BeTrue(), "Cast to v1alpha1.CFApp failed")
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
					_, reconcileErr = cfAppReconciler.Reconcile(ctx, req)
				})

				It("return the error", func() {
					Expect(reconcileErr).To(MatchError("failed to list CFRoute"))
				})
			})

			When("patching cfRoute returns an error", func() {
				BeforeEach(func() {
					cfRoutePatchErr = errors.New("failed to patch CFRoute")
					_, reconcileErr = cfAppReconciler.Reconcile(ctx, req)
				})

				It("return the error", func() {
					Expect(reconcileErr).To(MatchError("failed to patch CFRoute"))
				})
			})

			When("patching cfApp returns an error", func() {
				BeforeEach(func() {
					cfAppPatchErr = errors.New("failed to patch CFApp")
					_, reconcileErr = cfAppReconciler.Reconcile(ctx, req)
				})

				It("return the error", func() {
					Expect(reconcileErr).To(MatchError("failed to patch CFApp"))
				})
			})
		})
	})
})
