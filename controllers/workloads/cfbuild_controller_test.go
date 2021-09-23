package workloads_test

import (
	cfconfig "code.cloudfoundry.org/cf-k8s-controllers/config/cf"
	"context"
	"errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"testing"

	workloadsv1alpha1 "code.cloudfoundry.org/cf-k8s-controllers/apis/workloads/v1alpha1"
	. "code.cloudfoundry.org/cf-k8s-controllers/controllers/workloads"
	"code.cloudfoundry.org/cf-k8s-controllers/controllers/workloads/fake"
	. "code.cloudfoundry.org/cf-k8s-controllers/controllers/workloads/testutils"

	. "github.com/onsi/gomega"
	buildv1alpha1 "github.com/pivotal/kpack/pkg/apis/build/v1alpha1"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

func TestBuildReconciler(t *testing.T) {
	spec.Run(t, "CFBuildReconciler", testCFBuildReconciler, spec.Report(report.Terminal{}))

}

func testCFBuildReconciler(t *testing.T, when spec.G, it spec.S) {
	g := NewWithT(t)

	const (
		defaultNamespace       = "default"
		stagingConditionType   = "Staging"
		readyConditionType     = "Ready"
		succeededConditionType = "Succeeded"
	)

	var (
		fakeClient       *fake.CFClient
		fakeStatusWriter *fake.StatusWriter

		cfAppGUID     string
		cfPackageGUID string
		cfBuildGUID   string

		getCFBuild         *workloadsv1alpha1.CFBuild
		getCFBuildError    error
		getCFApp           *workloadsv1alpha1.CFApp
		getCFAppError      error
		getCFPackage       *workloadsv1alpha1.CFPackage
		getCFPackageError  error
		getKpackImageError error
		cfBuildReconciler  *CFBuildReconciler
		req                ctrl.Request
		ctx                context.Context

		reconcileResult ctrl.Result
		reconcileErr    error
	)

	// Set up happy path
	it.Before(func() {
		fakeClient = new(fake.CFClient)

		cfAppGUID = "cf-app-guid"
		cfPackageGUID = "cf-package-guid"
		cfBuildGUID = "cf-build-guid"

		getCFBuild = InitializeCFBuild(cfBuildGUID, defaultNamespace, cfPackageGUID, cfAppGUID)
		getCFBuildError = nil
		getCFApp = InitializeAppCR(cfAppGUID, defaultNamespace)
		getCFAppError = nil
		getCFPackage = InitializePackageCR(cfPackageGUID, defaultNamespace, cfAppGUID)
		getCFPackageError = nil
		getKpackImageError = apierrors.NewNotFound(schema.GroupResource{}, getCFBuild.Name)

		fakeClient.GetStub = func(_ context.Context, _ types.NamespacedName, obj client.Object) error {
			// cast obj to find its kind
			switch obj.(type) {
			case *workloadsv1alpha1.CFBuild:
				getCFBuild.DeepCopyInto(obj.(*workloadsv1alpha1.CFBuild))
				return getCFBuildError
			case *workloadsv1alpha1.CFApp:
				getCFApp.DeepCopyInto(obj.(*workloadsv1alpha1.CFApp))
				return getCFAppError
			case *workloadsv1alpha1.CFPackage:
				getCFPackage.DeepCopyInto(obj.(*workloadsv1alpha1.CFPackage))
				return getCFPackageError
			case *buildv1alpha1.Image:
				return getKpackImageError
			default:
				panic("test Client Get provided a weird obj")
			}
		}

		// Configure mock status update to succeed
		fakeStatusWriter = &fake.StatusWriter{}
		fakeClient.StatusReturns(fakeStatusWriter)

		// configure a CFBuildReconciler with the client
		g.Expect(workloadsv1alpha1.AddToScheme(scheme.Scheme)).To(Succeed())
		g.Expect(buildv1alpha1.AddToScheme(scheme.Scheme)).To(Succeed())
		cfBuildReconciler = &CFBuildReconciler{
			Client:           fakeClient,
			Scheme:           scheme.Scheme,
			Log:              zap.New(zap.WriteTo(it.Out()), zap.UseDevMode(true)),
			ControllerConfig: &cfconfig.ControllerConfig{KpackImageTag: "image/registry/tag"},
		}
		req = ctrl.Request{
			NamespacedName: types.NamespacedName{
				Namespace: defaultNamespace,
				Name:      cfBuildGUID,
			},
		}
		ctx = context.Background()
	})

	when("CFBuild status conditions are unknown and", func() {
		when("on the happy path", func() {

			it.Before(func() {
				reconcileResult, reconcileErr = cfBuildReconciler.Reconcile(ctx, req)
			})

			it("does not return an error", func() {
				g.Expect(reconcileErr).NotTo(HaveOccurred())
			})

			it("returns an empty result", func() {
				g.Expect(reconcileResult).To(Equal(ctrl.Result{}))
			})

			it("should create kpack image with the same GUID as the CF Build", func() {
				g.Expect(fakeClient.CreateCallCount()).To(Equal(1), "fakeClient Create was not called 1 time")
				_, kpackImage, _ := fakeClient.CreateArgsForCall(0)
				g.Expect(kpackImage.GetName()).To(Equal(cfBuildGUID))
			})

		})
		when("on unhappy path", func() {
			when("fetch CFBuild returns an error", func() {
				it.Before(func() {
					getCFBuildError = errors.New("failing on purpose")
					reconcileResult, reconcileErr = cfBuildReconciler.Reconcile(ctx, req)
				})
				it("should return an error", func() {
					g.Expect(reconcileErr).To(HaveOccurred())
				})
			})
			when("fetch CFBuild returns a NotFoundError", func() {
				it.Before(func() {
					getCFBuildError = apierrors.NewNotFound(schema.GroupResource{}, getCFBuild.Name)
					reconcileResult, reconcileErr = cfBuildReconciler.Reconcile(ctx, req)
				})
				it("should NOT return any error", func() {
					g.Expect(reconcileErr).NotTo(HaveOccurred())
				})
			})
			when("fetch CFApp returns an error", func() {
				it.Before(func() {
					getCFAppError = errors.New("failing on purpose")
					reconcileResult, reconcileErr = cfBuildReconciler.Reconcile(ctx, req)
				})
				it("should return an error", func() {
					g.Expect(reconcileErr).To(HaveOccurred())
				})
			})
			when("fetch CFPackage returns an error", func() {
				it.Before(func() {
					getCFPackageError = errors.New("failing on purpose")
					reconcileResult, reconcileErr = cfBuildReconciler.Reconcile(ctx, req)
				})
				it("should return an error", func() {
					g.Expect(reconcileErr).To(HaveOccurred())
				})
			})
			when("create Kpack Image returns an error", func() {
				it.Before(func() {
					fakeClient.CreateReturns(errors.New("failing on purpose"))
					reconcileResult, reconcileErr = cfBuildReconciler.Reconcile(ctx, req)
				})
				it("should return an error", func() {
					g.Expect(reconcileErr).To(HaveOccurred())
				})
			})
			when("update status conditions returns an error", func() {
				it.Before(func() {
					fakeStatusWriter.UpdateReturns(errors.New("failing on purpose"))
					reconcileResult, reconcileErr = cfBuildReconciler.Reconcile(ctx, req)
				})
				it("should return an error", func() {
					g.Expect(reconcileErr).To(HaveOccurred())
				})
			})
		})

	})
	when("CFBuild status conditions for Staging is True and others are unknown", func() {
		it.Before(func() {
			SetStatusCondition(&getCFBuild.Status.Conditions, StagingConditionType, metav1.ConditionTrue)
			SetStatusCondition(&getCFBuild.Status.Conditions, ReadyConditionType, metav1.ConditionUnknown)
			SetStatusCondition(&getCFBuild.Status.Conditions, SucceededConditionType, metav1.ConditionUnknown)
			reconcileResult, reconcileErr = cfBuildReconciler.Reconcile(ctx, req)
		})

		it("does not return an error", func() {
			g.Expect(reconcileErr).NotTo(HaveOccurred())
		})

		it("returns an empty result", func() {
			g.Expect(reconcileResult).To(Equal(ctrl.Result{}))
		})

		it("does not create a new kpack image", func() {
			g.Expect(fakeClient.CreateCallCount()).To(Equal(0), "fakeClient Create was called when it should not have been")
		})
	})

}
