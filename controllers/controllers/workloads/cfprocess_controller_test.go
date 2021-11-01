package workloads_test

import (
	workloadsv1alpha1 "code.cloudfoundry.org/cf-k8s-controllers/apis/workloads/v1alpha1"
	. "code.cloudfoundry.org/cf-k8s-controllers/controllers/workloads"
	"code.cloudfoundry.org/cf-k8s-controllers/controllers/workloads/fake"
	. "code.cloudfoundry.org/cf-k8s-controllers/controllers/workloads/testutils"
	eiriniv1 "code.cloudfoundry.org/eirini-controller/pkg/apis/eirini/v1"
	"context"
	"errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	. "github.com/onsi/ginkgo"
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

var _ = Describe("CFRouteReconciler Unit Tests", func() {

	var (
		fakeClient *fake.Client

		cfBuild   *workloadsv1alpha1.CFBuild
		cfProcess *workloadsv1alpha1.CFProcess
		cfApp     *workloadsv1alpha1.CFApp
		lrp       *eiriniv1.LRP

		cfBuildError      error
		cfAppError        error
		cfProcessError    error
		appEnvSecretError error
		lrpError          error

		cfProcessReconciler *CFProcessReconciler
		ctx                 context.Context
		req                 ctrl.Request

		reconcileErr error
	)

	BeforeEach(func() {
		fakeClient = new(fake.Client)

		cfApp = BuildCFAppCRObject(testAppGUID, testNamespace)
		cfAppError = nil
		cfBuild = BuildCFBuildObject(testBuildGUID, testNamespace, testPackageGUID, testAppGUID)
		UpdateCFBuildWithDropletStatus(cfBuild)
		cfBuildError = nil
		cfProcess = BuildCFProcessCRObject(testProcessGUID, testNamespace, testAppGUID, testProcessType, testProcessCommand)
		cfProcessError = nil
		appEnvSecretError = nil
		lrpError = nil

		fakeClient.GetStub = func(_ context.Context, _ types.NamespacedName, obj client.Object) error {
			// cast obj to find its kind
			switch obj.(type) {
			case *workloadsv1alpha1.CFProcess:
				cfProcess.DeepCopyInto(obj.(*workloadsv1alpha1.CFProcess))
				return cfProcessError
			case *workloadsv1alpha1.CFBuild:
				cfBuild.DeepCopyInto(obj.(*workloadsv1alpha1.CFBuild))
				return cfBuildError
			case *workloadsv1alpha1.CFApp:
				cfApp.DeepCopyInto(obj.(*workloadsv1alpha1.CFApp))
				return cfAppError
			case *corev1.Secret:
				return appEnvSecretError
			case *eiriniv1.LRP:
				lrp.DeepCopyInto(obj.(*eiriniv1.LRP))
				return lrpError
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
		When("one the happy path", func() {
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

				It("should return an error", func() {
					Expect(reconcileErr).To(MatchError(failsOnPurposeErrorMessage))
				})
			})
			When("fetch CFProcess returns a NotFoundError", func() {
				BeforeEach(func() {
					cfProcessError = apierrors.NewNotFound(schema.GroupResource{}, cfProcess.Name)
					_, reconcileErr = cfProcessReconciler.Reconcile(ctx, req)
				})

				It("should NOT return an error", func() {
					Expect(reconcileErr).NotTo(HaveOccurred())
				})
			})

			When("fetch CFApp returns an error", func() {
				BeforeEach(func() {
					cfAppError = errors.New(failsOnPurposeErrorMessage)
					_, reconcileErr = cfProcessReconciler.Reconcile(ctx, req)
				})

				It("should return an error", func() {
					Expect(reconcileErr).To(MatchError(failsOnPurposeErrorMessage))
				})
			})

			When("fetch CFBuild returns an error", func() {
				BeforeEach(func() {
					cfBuildError = errors.New(failsOnPurposeErrorMessage)
					_, reconcileErr = cfProcessReconciler.Reconcile(ctx, req)
				})

				It("should return an error", func() {
					Expect(reconcileErr).To(MatchError(failsOnPurposeErrorMessage))
				})
			})

			When("CFBuild does not have a build droplet status", func() {
				BeforeEach(func() {
					cfBuild.Status.BuildDropletStatus = nil
					_, reconcileErr = cfProcessReconciler.Reconcile(ctx, req)
				})

				It("should return an error", func() {
					Expect(reconcileErr).To(MatchError("no build droplet status on CFBuild"))
				})
			})

			When("fetch Secret returns an error", func() {
				BeforeEach(func() {
					appEnvSecretError = errors.New(failsOnPurposeErrorMessage)
					_, reconcileErr = cfProcessReconciler.Reconcile(ctx, req)
				})

				It("should return an error", func() {
					Expect(reconcileErr).To(MatchError(failsOnPurposeErrorMessage))
				})
			})
		})
	})
})
