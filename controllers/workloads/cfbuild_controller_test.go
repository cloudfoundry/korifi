package workloads_test

import (
	"context"
	"errors"
	"testing"

	"github.com/pivotal/kpack/pkg/apis/core/v1alpha1"
	corev1 "k8s.io/api/core/v1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	cfconfig "code.cloudfoundry.org/cf-k8s-controllers/config/cf"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"

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
		defaultNamespace        = "default"
		stagingConditionType    = "Staging"
		readyConditionType      = "Ready"
		succeededConditionType  = "Succeeded"
		kpackReadyConditionType = "Ready"
	)

	var (
		fakeClient       *fake.CFClient
		fakeStatusWriter *fake.StatusWriter

		cfAppGUID      string
		cfPackageGUID  string
		cfBuildGUID    string
		kpackImageGUID string

		cfBuild           *workloadsv1alpha1.CFBuild
		cfBuildError      error
		cfApp             *workloadsv1alpha1.CFApp
		cfAppError        error
		cfPackage         *workloadsv1alpha1.CFPackage
		cfPackageError    error
		kpackImage        *buildv1alpha1.Image
		kpackImageError   error
		cfBuildReconciler *CFBuildReconciler
		req               ctrl.Request
		ctx               context.Context

		reconcileResult ctrl.Result
		reconcileErr    error
	)

	it.Before(func() {
		fakeClient = new(fake.CFClient)

		cfAppGUID = "cf-app-guid"
		cfPackageGUID = "cf-package-guid"
		cfBuildGUID = "cf-build-guid"
		kpackImageGUID = cfBuildGUID

		cfBuild = MockCFBuildObject(cfBuildGUID, defaultNamespace, cfPackageGUID, cfAppGUID)
		cfBuildError = nil
		cfApp = MockAppCRObject(cfAppGUID, defaultNamespace)
		cfAppError = nil
		cfPackage = MockPackageCRObject(cfPackageGUID, defaultNamespace, cfAppGUID)
		cfPackageError = nil
		kpackImage = mockKpackImageObject(kpackImageGUID, defaultNamespace)
		kpackImageError = apierrors.NewNotFound(schema.GroupResource{}, cfBuild.Name)

		fakeClient.GetStub = func(_ context.Context, _ types.NamespacedName, obj client.Object) error {
			// cast obj to find its kind
			switch obj.(type) {
			case *workloadsv1alpha1.CFBuild:
				cfBuild.DeepCopyInto(obj.(*workloadsv1alpha1.CFBuild))
				return cfBuildError
			case *workloadsv1alpha1.CFApp:
				cfApp.DeepCopyInto(obj.(*workloadsv1alpha1.CFApp))
				return cfAppError
			case *workloadsv1alpha1.CFPackage:
				cfPackage.DeepCopyInto(obj.(*workloadsv1alpha1.CFPackage))
				return cfPackageError
			case *buildv1alpha1.Image:
				kpackImage.DeepCopyInto(obj.(*buildv1alpha1.Image))
				return kpackImageError
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
			it("should update the status conditions on CFBuild", func() {
				g.Expect(fakeClient.StatusCallCount()).To(Equal(1))
			})

		})
		when("on unhappy path", func() {
			when("fetch CFBuild returns an error", func() {
				it.Before(func() {
					cfBuildError = errors.New("failing on purpose")
					reconcileResult, reconcileErr = cfBuildReconciler.Reconcile(ctx, req)
				})
				it("should return an error", func() {
					g.Expect(reconcileErr).To(HaveOccurred())
				})
			})
			when("fetch CFBuild returns a NotFoundError", func() {
				it.Before(func() {
					cfBuildError = apierrors.NewNotFound(schema.GroupResource{}, cfBuild.Name)
					reconcileResult, reconcileErr = cfBuildReconciler.Reconcile(ctx, req)
				})
				it("should NOT return any error", func() {
					g.Expect(reconcileErr).NotTo(HaveOccurred())
				})
			})
			when("fetch CFApp returns an error", func() {
				it.Before(func() {
					cfAppError = errors.New("failing on purpose")
					reconcileResult, reconcileErr = cfBuildReconciler.Reconcile(ctx, req)
				})
				it("should return an error", func() {
					g.Expect(reconcileErr).To(HaveOccurred())
				})
			})
			when("fetch CFPackage returns an error", func() {
				it.Before(func() {
					cfPackageError = errors.New("failing on purpose")
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
			SetStatusCondition(&cfBuild.Status.Conditions, stagingConditionType, metav1.ConditionTrue)
			SetStatusCondition(&cfBuild.Status.Conditions, readyConditionType, metav1.ConditionUnknown)
			SetStatusCondition(&cfBuild.Status.Conditions, succeededConditionType, metav1.ConditionUnknown)
		})
		when("on the happy path", func() {
			it.Before(func() {
				kpackImageError = nil
				setKpackImageStatus(kpackImage, kpackReadyConditionType, "True")
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

			it("should update the status conditions on CFBuild", func() {
				g.Expect(fakeClient.StatusCallCount()).To(Equal(1))
			})
		})
		when("on the unhappy path", func() {
			when("fetch KpackImage returns an error", func() {
				it.Before(func() {
					kpackImageError = errors.New("failing on purpose")
					reconcileResult, reconcileErr = cfBuildReconciler.Reconcile(ctx, req)
				})
				it("should return an error", func() {
					g.Expect(reconcileErr).To(HaveOccurred())
				})
			})
			when("fetch KpackImage returns a NotFoundError", func() {
				it.Before(func() {
					kpackImageError = apierrors.NewNotFound(schema.GroupResource{}, cfBuild.Name)
					reconcileResult, reconcileErr = cfBuildReconciler.Reconcile(ctx, req)
				})
				it("should NOT return any error", func() {
					g.Expect(reconcileErr).NotTo(HaveOccurred())
				})
			})
			when("kpack image status conditions for Type Succeeded is nil", func() {
				it.Before(func() {
					reconcileResult, reconcileErr = cfBuildReconciler.Reconcile(ctx, req)
				})
				it("does not return an error", func() {
					g.Expect(reconcileErr).NotTo(HaveOccurred())
				})

				it("returns an empty result", func() {
					g.Expect(reconcileResult).To(Equal(ctrl.Result{}))
				})
			})
			when("kpack image status conditions for Type Succeeded is UNKNOWN", func() {
				it.Before(func() {
					kpackImageError = nil
					setKpackImageStatus(kpackImage, kpackReadyConditionType, "Unknown")
					reconcileResult, reconcileErr = cfBuildReconciler.Reconcile(ctx, req)
				})
				it("does not return an error", func() {
					g.Expect(reconcileErr).NotTo(HaveOccurred())
				})

				it("returns an empty result", func() {
					g.Expect(reconcileResult).To(Equal(ctrl.Result{}))
				})
			})
			when("kpack image status conditions for Type Succeeded is FALSE", func() {
				it.Before(func() {
					kpackImageError = nil
					setKpackImageStatus(kpackImage, kpackReadyConditionType, "False")
					reconcileResult, reconcileErr = cfBuildReconciler.Reconcile(ctx, req)
				})
				it("does not return an error", func() {
					g.Expect(reconcileErr).NotTo(HaveOccurred())
				})
				it("updates status conditions on CFBuild", func() {
					g.Expect(fakeClient.StatusCallCount()).To(Equal(1))
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
			when("kpack image status conditions for Type Succeeded is True", func() {
				when("update status conditions returns an error", func() {
					it.Before(func() {
						kpackImageError = nil
						setKpackImageStatus(kpackImage, kpackReadyConditionType, "True")
						fakeStatusWriter.UpdateReturns(errors.New("failing on purpose"))
						reconcileResult, reconcileErr = cfBuildReconciler.Reconcile(ctx, req)
					})
					it("should return an error", func() {
						g.Expect(reconcileErr).To(HaveOccurred())
					})
				})
			})
		})
	})

}

func mockKpackImageObject(guid string, namespace string) *buildv1alpha1.Image {
	return &buildv1alpha1.Image{
		ObjectMeta: metav1.ObjectMeta{
			Name:      guid,
			Namespace: namespace,
		},
		Spec: buildv1alpha1.ImageSpec{
			Tag:            "test-tag",
			Builder:        corev1.ObjectReference{},
			ServiceAccount: "test-service-account",
			Source: buildv1alpha1.SourceConfig{
				Registry: &buildv1alpha1.Registry{
					Image:            "image-path",
					ImagePullSecrets: nil,
				},
			},
		},
	}
}

func setKpackImageStatus(kpackImage *buildv1alpha1.Image, conditionType string, conditionStatus string) {
	kpackImage.Status.Conditions = append(kpackImage.Status.Conditions, v1alpha1.Condition{
		Type:   v1alpha1.ConditionType(conditionType),
		Status: corev1.ConditionStatus(conditionStatus),
	})
}
