package workloads_test

import (
	"context"
	"fmt"

	workloadsv1alpha1 "code.cloudfoundry.org/cf-k8s-controllers/apis/workloads/v1alpha1"
	. "code.cloudfoundry.org/cf-k8s-controllers/controllers/workloads"
	"code.cloudfoundry.org/cf-k8s-controllers/controllers/workloads/fake"

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
	dummyCFAppName      = "dummy"
	dummyCFAppNamespace = "default"

	getErrorMessage          = "Get fails on purpose!"
	statusUpdateErrorMessage = "Update fails on purpose!"
)

var _ = Describe("CFAppReconciler", func() {
	var (
		fakeClient      *fake.CFClient
		cfAppReconciler *CFAppReconciler
		ctx             context.Context
		req             ctrl.Request
	)

	BeforeEach(func() {
		fakeClient = new(fake.CFClient)
		// configure a CFAppReconciler with the client
		Expect(workloadsv1alpha1.AddToScheme(scheme.Scheme)).To(Succeed())
		cfAppReconciler = &CFAppReconciler{
			Client: fakeClient,
			Scheme: scheme.Scheme,
			Log:    zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)),
		}
		ctx = context.Background()
		req = ctrl.Request{
			NamespacedName: types.NamespacedName{
				Namespace: dummyCFAppNamespace,
				Name:      dummyCFAppName,
			},
		}
	})

	When("The CFAppReconciler Reconcile function is called", func() {
		var fakeStatusWriter *fake.StatusWriter

		BeforeEach(func() {
			// Tell get to return a nice CFApp
			// Configure the mock fakeClient.Get() to return the expected app
			fakeClient.GetStub = func(ctx context.Context, name types.NamespacedName, object client.Object) error {
				cast := object.(*workloadsv1alpha1.CFApp)
				cast.ObjectMeta.Name = dummyCFAppName
				cast.ObjectMeta.Namespace = dummyCFAppNamespace
				return nil
			}
			// Configure mock status update to succeed
			fakeStatusWriter = &fake.StatusWriter{}
			fakeClient.StatusReturns(fakeStatusWriter)

			// Have status validate inputs
			// Have status return no error
		})

		It("returns an empty result and and nil", func() {
			result, err := cfAppReconciler.Reconcile(ctx, req)
			Expect(result).To(Equal(ctrl.Result{}))
			Expect(err).NotTo(HaveOccurred())

			// validate the inputs to Get
			Expect(fakeClient.GetCallCount()).To(Equal(1))
			_, testRequestNamespacedName, _ := fakeClient.GetArgsForCall(0)
			Expect(testRequestNamespacedName.Namespace).To(Equal(dummyCFAppNamespace))
			Expect(testRequestNamespacedName.Name).To(Equal(dummyCFAppName))

			// validate the inputs to Status.Update
			Expect(fakeStatusWriter.UpdateCallCount()).To(Equal(1))
			_, updatedCFApp, _ := fakeStatusWriter.UpdateArgsForCall(0)
			cast, ok := updatedCFApp.(*workloadsv1alpha1.CFApp)
			Expect(ok).To(BeTrue(), "Cast to workloadsv1alpha1.CFApp failed")
			Expect(meta.IsStatusConditionFalse(cast.Status.Conditions, StatusConditionRunning)).To(BeTrue(), "Status Condition "+StatusConditionRunning+" was not False as expected")
			Expect(meta.IsStatusConditionFalse(cast.Status.Conditions, StatusConditionRestarting)).To(BeTrue(), "Status Condition "+StatusConditionRestarting+" was not False as expected")
		})
	})

	When("The CFAppReconciler is configured with an CFApp Client where Get() will fail", func() {
		BeforeEach(func() {
			// Configure the mock fakeClient.Get() to return an error
			fakeClient.GetReturns(fmt.Errorf(getErrorMessage))
		})

		It("returns an error", func() {
			_, err := cfAppReconciler.Reconcile(ctx, req)
			Expect(err).To(MatchError(getErrorMessage))
		})
	})

	When("The CFAppReconciler is configured with an CFApp Client where Status().Update() will fail", func() {
		BeforeEach(func() {
			// Configure the mock fakeClient.Get() to return the expected app
			fakeClient.GetStub = func(ctx context.Context, name types.NamespacedName, object client.Object) error {
				cast := object.(*workloadsv1alpha1.CFApp)
				cast.ObjectMeta.Name = dummyCFAppName
				cast.ObjectMeta.Namespace = dummyCFAppNamespace
				return nil
			}

			// Configure mock status update to fail
			fakeStatusWriter := &fake.StatusWriter{}
			fakeStatusWriter.UpdateReturns(fmt.Errorf(statusUpdateErrorMessage))
			fakeClient.StatusReturns(fakeStatusWriter)
		})

		It("returns an error", func() {
			_, err := cfAppReconciler.Reconcile(ctx, req)
			Expect(err).To(MatchError(statusUpdateErrorMessage))
		})
	})
})
