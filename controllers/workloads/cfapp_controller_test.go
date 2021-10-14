package workloads_test

import (
	"context"
	"errors"

	config "code.cloudfoundry.org/cf-k8s-controllers/config/base"

	v1 "k8s.io/api/core/v1"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"

	workloadsv1alpha1 "code.cloudfoundry.org/cf-k8s-controllers/apis/workloads/v1alpha1"
	. "code.cloudfoundry.org/cf-k8s-controllers/controllers/workloads"
	"code.cloudfoundry.org/cf-k8s-controllers/controllers/workloads/fake"
	. "code.cloudfoundry.org/cf-k8s-controllers/controllers/workloads/testutils"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/meta"
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
		fakeClient       *fake.CFClient
		fakeStatusWriter *fake.StatusWriter

		cfAppGUID     string
		cfBuildGUID   string
		cfPackageGUID string

		cfBuild      *workloadsv1alpha1.CFBuild
		cfBuildError error
		cfApp        *workloadsv1alpha1.CFApp
		cfAppError   error

		cfAppReconciler *CFAppReconciler
		ctx             context.Context
		req             ctrl.Request

		reconcileResult ctrl.Result
		reconcileErr    error
	)

	BeforeEach(func() {
		fakeClient = new(fake.CFClient)

		cfAppGUID = "cf-app-guid"
		cfPackageGUID = "cf-package-guid"
		cfBuildGUID = "cf-build-guid"

		cfApp = BuildCFAppCRObject(cfAppGUID, defaultNamespace)
		cfAppError = nil
		cfBuild = BuildCFBuildObject(cfBuildGUID, defaultNamespace, cfPackageGUID, cfAppGUID)
		updateCFBuildWithDropletStatus(cfBuild)
		cfBuildError = nil

		fakeClient.GetStub = func(_ context.Context, _ types.NamespacedName, obj client.Object) error {
			// cast obj to find its kind
			switch obj.(type) {
			case *workloadsv1alpha1.CFBuild:
				cfBuild.DeepCopyInto(obj.(*workloadsv1alpha1.CFBuild))
				return cfBuildError
			case *workloadsv1alpha1.CFApp:
				cfApp.DeepCopyInto(obj.(*workloadsv1alpha1.CFApp))
				return cfAppError
			default:
				panic("TestClient Get provided a weird obj")
			}
		}

		// Configure mock status update to succeed
		fakeStatusWriter = &fake.StatusWriter{}
		fakeClient.StatusReturns(fakeStatusWriter)

		cfProcessList := workloadsv1alpha1.CFProcessList{}
		fakeClient.ListStub = func(ctx context.Context, list client.ObjectList, option ...client.ListOption) error {
			cfProcessList.DeepCopyInto(list.(*workloadsv1alpha1.CFProcessList))
			return nil
		}

		// configure a CFAppReconciler with the client
		Expect(workloadsv1alpha1.AddToScheme(scheme.Scheme)).To(Succeed())
		cfAppReconciler = &CFAppReconciler{
			Client: fakeClient,
			Scheme: scheme.Scheme,
			Log:    zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)),
			ControllerConfig: &config.ControllerConfig{
				KpackImageTag: "image/registry/tag",
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
				cast, ok := updatedCFApp.(*workloadsv1alpha1.CFApp)
				Expect(ok).To(BeTrue(), "Cast to workloadsv1alpha1.CFApp failed")
				Expect(meta.IsStatusConditionFalse(cast.Status.Conditions, StatusConditionRunning)).To(BeTrue(), "Status Condition "+StatusConditionRunning+" was not False as expected")
				Expect(meta.IsStatusConditionFalse(cast.Status.Conditions, StatusConditionRestarting)).To(BeTrue(), "Status Condition "+StatusConditionRestarting+" was not False as expected")
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

				//Validate args to fetch CFApp
				_, testRequestNamespacedName, _ := fakeClient.GetArgsForCall(0)
				Expect(testRequestNamespacedName.Namespace).To(Equal(defaultNamespace))
				Expect(testRequestNamespacedName.Name).To(Equal(cfAppGUID))

				//Validate args to fetch CFBuild
				_, testRequestNamespacedName, _ = fakeClient.GetArgsForCall(1)
				Expect(testRequestNamespacedName.Namespace).To(Equal(defaultNamespace))
				Expect(testRequestNamespacedName.Name).To(Equal(cfBuildGUID))

				//Validate call count to fetch CFProcess
				Expect(fakeClient.ListCallCount()).To(Equal(1))

				//Validate call count to create CFProcess
				Expect(fakeClient.CreateCallCount()).To(Equal(1))

				// validate the inputs to Status.Update
				Expect(fakeStatusWriter.UpdateCallCount()).To(Equal(1))
				_, updatedCFApp, _ := fakeStatusWriter.UpdateArgsForCall(0)
				cast, ok := updatedCFApp.(*workloadsv1alpha1.CFApp)
				Expect(ok).To(BeTrue(), "Cast to workloadsv1alpha1.CFApp failed")
				Expect(meta.IsStatusConditionFalse(cast.Status.Conditions, StatusConditionRunning)).To(BeTrue(), "Status Condition "+StatusConditionRunning+" was not False as expected")
				Expect(meta.IsStatusConditionFalse(cast.Status.Conditions, StatusConditionRestarting)).To(BeTrue(), "Status Condition "+StatusConditionRestarting+" was not False as expected")
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
					cfBuild.Status.BuildDropletStatus = nil
					_, reconcileErr = cfAppReconciler.Reconcile(ctx, req)
				})

				It("should returns an error", func() {
					Expect(reconcileErr).To(MatchError(ContainSubstring(buildDropletStatusErrorMessage)))
				})
			})

			When("Label value doesnt conform to the syntax", func() {
				BeforeEach(func() {
					cfBuild.Status.BuildDropletStatus.ProcessTypes[0].Type = "#web"
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
		})
	})

})

func updateCFBuildWithDropletStatus(cfbuild *workloadsv1alpha1.CFBuild) {
	cfbuild.Status.BuildDropletStatus = &workloadsv1alpha1.BuildDropletStatus{
		Registry: workloadsv1alpha1.Registry{
			Image:            "my-image",
			ImagePullSecrets: nil,
		},
		Stack: "cflinuxfs3",
		ProcessTypes: []workloadsv1alpha1.ProcessType{
			{
				Type:    "web",
				Command: "web-command",
			},
		},
		Ports: []int32{8080},
	}
}
