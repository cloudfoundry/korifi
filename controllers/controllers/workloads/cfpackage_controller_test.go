package workloads_test

import (
	"context"
	"errors"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"

	workloadsv1alpha1 "code.cloudfoundry.org/cf-k8s-controllers/controllers/apis/workloads/v1alpha1"
	. "code.cloudfoundry.org/cf-k8s-controllers/controllers/controllers/workloads"
	"code.cloudfoundry.org/cf-k8s-controllers/controllers/controllers/workloads/fake"
	. "code.cloudfoundry.org/cf-k8s-controllers/controllers/controllers/workloads/testutils"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	buildv1alpha2 "github.com/pivotal/kpack/pkg/apis/build/v1alpha2"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var _ = Describe("CFPackageReconciler", func() {
	var (
		fakeClient *fake.Client

		cfAppGUID     string
		cfPackageGUID string

		cfApp                *workloadsv1alpha1.CFApp
		cfAppError           error
		cfPackage            *workloadsv1alpha1.CFPackage
		cfPackageError       error
		cfPackageUpdateError error

		cfPackageReconciler *CFPackageReconciler
		req                 ctrl.Request
		ctx                 context.Context

		reconcileResult ctrl.Result
		reconcileErr    error
	)

	BeforeEach(func() {
		fakeClient = new(fake.Client)

		cfAppGUID = "cf-app-guid"
		cfPackageGUID = "cf-package-guid"

		cfApp = BuildCFAppCRObject(cfAppGUID, defaultNamespace)
		cfAppError = nil
		cfPackage = BuildCFPackageCRObject(cfPackageGUID, defaultNamespace, cfAppGUID)
		cfPackageError = nil

		fakeClient.GetStub = func(_ context.Context, _ types.NamespacedName, obj client.Object) error {
			switch obj := obj.(type) {
			case *workloadsv1alpha1.CFApp:
				cfApp.DeepCopyInto(obj)
				return cfAppError
			case *workloadsv1alpha1.CFPackage:
				cfPackage.DeepCopyInto(obj)
				return cfPackageError

			default:
				panic("test Client Get provided a weird obj")
			}
		}

		cfPackageUpdateError = nil
		fakeClient.PatchStub = func(ctx context.Context, object client.Object, patch client.Patch, option ...client.PatchOption) error {
			return cfPackageUpdateError
		}

		Expect(workloadsv1alpha1.AddToScheme(scheme.Scheme)).To(Succeed())
		Expect(buildv1alpha2.AddToScheme(scheme.Scheme)).To(Succeed())
		cfPackageReconciler = &CFPackageReconciler{
			Client: fakeClient,
			Scheme: scheme.Scheme,
			Log:    zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)),
		}
		req = ctrl.Request{
			NamespacedName: types.NamespacedName{
				Namespace: defaultNamespace,
				Name:      cfPackageGUID,
			},
		}
		ctx = context.Background()
	})

	When("CFPackage is created and the CFPackageReconciler reconciles", func() {
		When("on the happy path", func() {
			BeforeEach(func() {
				reconcileResult, reconcileErr = cfPackageReconciler.Reconcile(ctx, req)
			})

			It("does not return an error", func() {
				Expect(reconcileErr).NotTo(HaveOccurred())
			})

			It("returns an empty result", func() {
				Expect(reconcileResult).To(Equal(ctrl.Result{}))
			})
		})

		When("on the unhappy path", func() {
			When("fetch CFPackage returns an error", func() {
				BeforeEach(func() {
					cfPackageError = errors.New("failing on purpose")
					reconcileResult, reconcileErr = cfPackageReconciler.Reconcile(ctx, req)
				})

				It("should return an error", func() {
					Expect(reconcileErr).To(HaveOccurred())
				})
			})

			When("fetch CFPackage returns a NotFoundError", func() {
				BeforeEach(func() {
					cfPackageError = apierrors.NewNotFound(schema.GroupResource{}, cfPackage.Name)
					reconcileResult, reconcileErr = cfPackageReconciler.Reconcile(ctx, req)
				})

				It("should NOT return any error", func() {
					Expect(reconcileErr).NotTo(HaveOccurred())
				})
			})

			When("fetch CFApp returns an error", func() {
				BeforeEach(func() {
					cfAppError = errors.New("failing on purpose")
					reconcileResult, reconcileErr = cfPackageReconciler.Reconcile(ctx, req)
				})

				It("should return an error", func() {
					Expect(reconcileErr).To(HaveOccurred())
				})
			})

			When("patch CFPackage returns an error", func() {
				BeforeEach(func() {
					cfPackageUpdateError = errors.New("failing on purpose")
					reconcileResult, reconcileErr = cfPackageReconciler.Reconcile(ctx, req)
				})

				It("should return an error", func() {
					Expect(reconcileErr).To(HaveOccurred())
				})
			})
		})
	})
})
