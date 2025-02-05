package runnerinfo_test

import (
	"context"
	"time"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/statefulset-runner/controllers/runnerinfo"
	"code.cloudfoundry.org/korifi/tools/k8s"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("RunnerInfo Reconcile", func() {
	var (
		reconciler      *k8s.PatchingReconciler[korifiv1alpha1.RunnerInfo, *korifiv1alpha1.RunnerInfo]
		reconcileResult ctrl.Result
		reconcileErr    error
		req             ctrl.Request
		runnerInfo      *korifiv1alpha1.RunnerInfo
	)

	BeforeEach(func() {
		Expect(korifiv1alpha1.AddToScheme(scheme.Scheme)).To(Succeed())

		runnerInfo = &korifiv1alpha1.RunnerInfo{
			ObjectMeta: v1.ObjectMeta{
				Name:       "statefulset-runner",
				Namespace:  uuid.NewString(),
				Generation: 1,
			},
			Spec: korifiv1alpha1.RunnerInfoSpec{
				RunnerName: "statefulset-runner",
			},
		}

		fakeClient.GetStub = func(_ context.Context, _ types.NamespacedName, obj client.Object, _ ...client.GetOption) error {
			switch obj := obj.(type) {
			case *korifiv1alpha1.RunnerInfo:
				runnerInfo.DeepCopyInto(obj)
				return nil
			default:
				panic("TestClient Get provided an unexpected object type")
			}
		}

		reconciler = runnerinfo.NewRunnerInfoReconciler(
			fakeClient,
			scheme.Scheme,
			ctrl.Log.WithName("controllers").WithName("TestRunnerInfo"),
		)
	})

	JustBeforeEach(func() {
		reconcileResult, reconcileErr = reconciler.Reconcile(context.Background(), req)
	})

	It("reconciles without error", func() {
		Expect(reconcileResult).To(Equal(ctrl.Result{}))
		Expect(reconcileErr).NotTo(HaveOccurred())
		_, object, _, _ := fakeStatusWriter.PatchArgsForCall(0)
		patchedRunnerInfo, ok := object.(*korifiv1alpha1.RunnerInfo)
		Expect(ok).To(BeTrue())
		Expect(patchedRunnerInfo.Status.ObservedGeneration).To(Equal(patchedRunnerInfo.Generation))
	})

	It("applies the Status.Capabilities.RollingDeploy field", func() {
		_, object, _, _ := fakeStatusWriter.PatchArgsForCall(0)
		patchedRunnerInfo, ok := object.(*korifiv1alpha1.RunnerInfo)
		Expect(ok).To(BeTrue())
		Expect(patchedRunnerInfo.Status.ObservedGeneration).To(Equal(patchedRunnerInfo.Generation))
		Expect(patchedRunnerInfo.Status.Capabilities.RollingDeploy).To(BeTrue())
	})

	When("the RunnerInfo is being deleted gracefully", func() {
		BeforeEach(func() {
			runnerInfo.DeletionTimestamp = &v1.Time{Time: time.Now()}
		})

		It("returns an empty result and does not return error", func() {
			Expect(reconcileResult).To(Equal(ctrl.Result{}))
			Expect(reconcileErr).NotTo(HaveOccurred())
		})

		It("does not reconcile the info", func() {
			Expect(fakeStatusWriter.PatchCallCount()).To(Equal(1))
			_, object, _, _ := fakeStatusWriter.PatchArgsForCall(0)
			patchedRunnerInfo, ok := object.(*korifiv1alpha1.RunnerInfo)
			Expect(ok).To(BeTrue())
			Expect(patchedRunnerInfo.Status.ObservedGeneration).To(BeZero())
		})
	})
})
