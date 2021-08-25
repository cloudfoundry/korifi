package reconcilers_test

import (
	"context"
	"fmt"
	. "github.com/onsi/gomega"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"testing"

	workloadsv1alpha1 "code.cloudfoundry.org/cf-k8s-controllers/api/v1alpha1"
	. "code.cloudfoundry.org/cf-k8s-controllers/controllers/reconcilers"
	"code.cloudfoundry.org/cf-k8s-controllers/controllers/reconcilers/reconcilersfakes"
)

func TestReconcilers(t *testing.T) {
	spec.Run(t, "object", testCFAppReconciler, spec.Report(report.Terminal{}))

}

func testCFAppReconciler(t *testing.T, when spec.G, it spec.S) {

	Expect := NewWithT(t).Expect

	when("The CFAppReconciler is configured with an CFApp Client where Get() will fail", func() {
		var (
			cfAppReconciler *CFAppReconciler
			ctx context.Context
			req ctrl.Request
		)
		it.Before(func() {
			// Create a mock CFAppClient
			client := new(reconcilersfakes.FakeCFAppClient)
			// Configure the mock client.Get() to return an error
			client.GetReturns(fmt.Errorf("Get fails on purpose!"))

			// configure a CFAppReconciler with the client
			err := workloadsv1alpha1.AddToScheme(scheme.Scheme)
			Expect(err).NotTo(HaveOccurred())
			cfAppReconciler = &CFAppReconciler{
				Client: client,
				Scheme: scheme.Scheme,
				Log:    zap.New(zap.WriteTo(it.Out()), zap.UseDevMode(true)),
			}

			// Set up a dummy request and context
			ctx = context.Background()
			req = ctrl.Request{
				NamespacedName: types.NamespacedName{
					Namespace: "default",
					Name:      "dummy",
				},
			}
		})

		it("returns an empty result and an error", func() {
			result, err := cfAppReconciler.Reconcile(ctx, req)
			Expect(result).To(Equal(ctrl.Result{}))
			Expect(err).To(HaveOccurred())

		})
	})

	when("The CFAppReconciler is configured with an CFApp Client where Status().Update() will fail", func() {
		var (
			cfAppReconciler *CFAppReconciler
			ctx             context.Context
			req             ctrl.Request
		)
		it.Before(func() {
			// Create a mock CFAppClient
			fakeClient := new(reconcilersfakes.FakeCFAppClient)
			// Configure the mock client.Get() to return an error
			fakeClient.GetStub = func(ctx context.Context, name types.NamespacedName, object client.Object) error {
				cast := object.(*workloadsv1alpha1.CFApp)
				cast.ObjectMeta.Name = "dummy"
				cast.ObjectMeta.Namespace = "default"
				return nil
			}

			fakeStatusWriter := &reconcilersfakes.FakeStatusWriter{}
			fakeStatusWriter.UpdateReturns(fmt.Errorf("Update fails on purpose!"))
			fakeClient.StatusReturns(fakeStatusWriter)

			// configure a CFAppReconciler with the client
			err := workloadsv1alpha1.AddToScheme(scheme.Scheme)
			Expect(err).NotTo(HaveOccurred())
			cfAppReconciler = &CFAppReconciler{
				Client: fakeClient,
				Scheme: scheme.Scheme,
				Log:    zap.New(zap.WriteTo(it.Out()), zap.UseDevMode(true)),
			}

			// Set up a dummy request and context
			ctx = context.Background()
			req = ctrl.Request{
				NamespacedName: types.NamespacedName{
					Namespace: "default",
					Name:      "dummy",
				},
			}
		})

		it("returns an empty result and an error", func() {
			result, err := cfAppReconciler.Reconcile(ctx, req)
			Expect(result).To(Equal(ctrl.Result{}))
			Expect(err).To(HaveOccurred())
		})
	})
}