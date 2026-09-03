package runnerinfo_test

import (
	"context"
	"time"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/knative-runner/controllers"
	"code.cloudfoundry.org/korifi/knative-runner/controllers/runnerinfo"
	"code.cloudfoundry.org/korifi/tools/k8s"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("RunnerInfo Reconcile", func() {
	var (
		reconciler      *k8s.PatchingReconciler[korifiv1alpha1.RunnerInfo]
		reconcileResult ctrl.Result
		reconcileErr    error
		req             ctrl.Request
		runnerInfo      *korifiv1alpha1.RunnerInfo
	)

	BeforeEach(func() {
		runnerInfo = &korifiv1alpha1.RunnerInfo{
			ObjectMeta: metav1.ObjectMeta{
				Name:       controllers.AppWorkloadReconcilerName,
				Namespace:  uuid.NewString(),
				Generation: 1,
			},
			Spec: korifiv1alpha1.RunnerInfoSpec{
				RunnerName: controllers.AppWorkloadReconcilerName,
			},
		}

		req = ctrl.Request{
			NamespacedName: types.NamespacedName{
				Name:      runnerInfo.Name,
				Namespace: runnerInfo.Namespace,
			},
		}

		fakeClient.GetStub = func(_ context.Context, _ types.NamespacedName, obj client.Object, _ ...client.GetOption) error {
			switch o := obj.(type) {
			case *korifiv1alpha1.RunnerInfo:
				runnerInfo.DeepCopyInto(o)
				return nil
			default:
				panic("unexpected Get object type")
			}
		}

		reconciler = runnerinfo.NewRunnerInfoReconciler(
			fakeClient,
			scheme.Scheme,
			ctrl.Log.WithName("controllers").WithName("TestKnativeRunnerInfo"),
		)
	})

	JustBeforeEach(func() {
		reconcileResult, reconcileErr = reconciler.Reconcile(context.Background(), req)
	})

	It("reconciles without error", func() {
		Expect(reconcileResult).To(Equal(ctrl.Result{}))
		Expect(reconcileErr).NotTo(HaveOccurred())
	})

	It("sets ObservedGeneration and RollingDeploy capability", func() {
		_, object, _, _ := fakeStatusWriter.PatchArgsForCall(0)
		patched := object.(*korifiv1alpha1.RunnerInfo)
		Expect(patched.Status.ObservedGeneration).To(Equal(patched.Generation))
		Expect(patched.Status.Capabilities.RollingDeploy).To(BeTrue())
	})

	When("the RunnerInfo is being deleted", func() {
		BeforeEach(func() {
			runnerInfo.DeletionTimestamp = &metav1.Time{Time: time.Now()}
		})

		It("returns success without setting capabilities", func() {
			Expect(reconcileErr).NotTo(HaveOccurred())
			Expect(reconcileResult).To(Equal(ctrl.Result{}))
			_, object, _, _ := fakeStatusWriter.PatchArgsForCall(0)
			patched := object.(*korifiv1alpha1.RunnerInfo)
			Expect(patched.Status.ObservedGeneration).To(BeZero())
		})
	})
})
