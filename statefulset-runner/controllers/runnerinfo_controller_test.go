package controllers_test

import (
	"context"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/statefulset-runner/controllers"
	"code.cloudfoundry.org/korifi/tools/k8s"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	testNamespace = "test-ns"
)

var _ = Describe("RunnerInfo Reconcile", func() {
	var (
		reconciler         *k8s.PatchingReconciler[korifiv1alpha1.RunnerInfo, *korifiv1alpha1.RunnerInfo]
		reconcileResult    ctrl.Result
		reconcileErr       error
		req                ctrl.Request
		getRunnerInfoError error
		runnerInfo         *korifiv1alpha1.RunnerInfo
		runnerName         string
	)

	JustBeforeEach(func() {
		Expect(korifiv1alpha1.AddToScheme(scheme.Scheme)).To(Succeed())

		runnerInfo = &korifiv1alpha1.RunnerInfo{
			ObjectMeta: v1.ObjectMeta{
				Name:      runnerName,
				Namespace: testNamespace,
			},
			Spec: korifiv1alpha1.RunnerInfoSpec{
				RunnerName: runnerName,
			},
		}

		getRunnerInfoError = nil

		fakeClient.GetStub = func(_ context.Context, _ types.NamespacedName, obj client.Object, _ ...client.GetOption) error {
			switch obj := obj.(type) {
			case *korifiv1alpha1.RunnerInfo:
				runnerInfo.DeepCopyInto(obj)
				return getRunnerInfoError
			default:
				panic("TestClient Get provided an unexpected object type")
			}
		}

		reconciler = controllers.NewRunnerInfoReconciler(
			fakeClient,
			scheme.Scheme,
			ctrl.Log.WithName("controllers").WithName("TestRunnerInfo"),
		)
		reconcileResult, reconcileErr = reconciler.Reconcile(context.Background(), req)
	})

	When("the RunnerInfo is being reconciled", func() {
		It("reconciles without error", func() {
			Expect(reconcileResult).To(Equal(ctrl.Result{}))
			Expect(reconcileErr).NotTo(HaveOccurred())
		})
	})

	// Filtering is done via predicate. This directly invokes the reconcile function, so the negative case cannot be tested here.
	When("the RunnerName matches the AppWorkloadReconcilerName", func() {
		BeforeEach(func() {
			runnerName = "statefulset-runner"
		})

		It("applies the Status.Capabilities.RollingDeploy field", func() {
			_, object, _, _ := fakeStatusWriter.PatchArgsForCall(0)
			patchedRunnerInfo, ok := object.(*korifiv1alpha1.RunnerInfo)
			Expect(ok).To(BeTrue())
			Expect(patchedRunnerInfo.Status.ObservedGeneration).To(Equal(patchedRunnerInfo.Generation))
			Expect(patchedRunnerInfo.Status.Capabilities.RollingDeploy).To(BeTrue())
		})
	})
})
