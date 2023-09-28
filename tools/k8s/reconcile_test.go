package k8s_test

import (
	"context"
	"errors"

	"github.com/go-logr/logr"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"code.cloudfoundry.org/korifi/controllers/fake"
	"code.cloudfoundry.org/korifi/tools/k8s"
)

type fakeObjectReconciler struct {
	reconcileResourceError     error
	reconcileResourceCallCount int
	reconcileResourceObj       *corev1.Pod
}

func (f *fakeObjectReconciler) ReconcileResource(ctx context.Context, obj *corev1.Pod) (ctrl.Result, error) {
	log := logr.FromContextOrDiscard(ctx)
	log.V(1).Info("fake reconciler reconciling")

	f.reconcileResourceCallCount++
	f.reconcileResourceObj = obj

	obj.Spec.RestartPolicy = corev1.RestartPolicyOnFailure
	obj.Status.Message = "hello"

	return ctrl.Result{
		RequeueAfter: 1,
	}, f.reconcileResourceError
}

func (f *fakeObjectReconciler) SetupWithManager(mgr ctrl.Manager) *builder.Builder {
	return nil
}

var _ = Describe("Reconcile", func() {
	var (
		ctx                context.Context
		fakeClient         *fake.Client
		fakeStatusWriter   *fake.StatusWriter
		patchingReconciler *k8s.PatchingReconciler[corev1.Pod, *corev1.Pod]
		objectReconciler   *fakeObjectReconciler
		pod                *corev1.Pod
		result             ctrl.Result
		err                error
	)

	BeforeEach(func() {
		objectReconciler = new(fakeObjectReconciler)
		fakeClient = new(fake.Client)
		fakeStatusWriter = new(fake.StatusWriter)
		fakeClient.StatusReturns(fakeStatusWriter)

		ctx = context.Background()
		pod = &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: uuid.NewString(),
				Name:      uuid.NewString(),
			},
			Spec:   corev1.PodSpec{},
			Status: corev1.PodStatus{},
		}

		fakeClient.PatchStub = func(_ context.Context, obj client.Object, _ client.Patch, _ ...client.PatchOption) error {
			o, ok := obj.(*corev1.Pod)
			Expect(ok).To(BeTrue())
			o.Status = corev1.PodStatus{}
			return nil
		}

		fakeClient.GetStub = func(_ context.Context, _ types.NamespacedName, obj client.Object, _ ...client.GetOption) error {
			o, ok := obj.(*corev1.Pod)
			Expect(ok).To(BeTrue())
			*o = *pod

			return nil
		}

		patchingReconciler = k8s.NewPatchingReconciler[corev1.Pod, *corev1.Pod](ctrl.Log, fakeClient, objectReconciler)
	})

	JustBeforeEach(func() {
		result, err = patchingReconciler.Reconcile(ctx, ctrl.Request{
			NamespacedName: types.NamespacedName{
				Namespace: pod.Namespace,
				Name:      pod.Name,
			},
		})
	})

	It("fetches the object", func() {
		Expect(fakeClient.GetCallCount()).To(Equal(1))
		_, namespacedName, obj, _ := fakeClient.GetArgsForCall(0)
		Expect(namespacedName.Namespace).To(Equal(pod.Namespace))
		Expect(namespacedName.Name).To(Equal(pod.Name))
		Expect(obj).To(BeAssignableToTypeOf(&corev1.Pod{}))
	})

	When("the object does not exist", func() {
		BeforeEach(func() {
			fakeClient.GetReturns(apierrors.NewNotFound(schema.GroupResource{}, "pod"))
		})

		It("does not call the object reconciler and succeeds", func() {
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(ctrl.Result{}))
			Expect(objectReconciler.reconcileResourceCallCount).To(Equal(0))
		})
	})

	When("the getting the object fails", func() {
		BeforeEach(func() {
			fakeClient.GetReturns(errors.New("get-error"))
		})

		It("fails without calling the reconciler", func() {
			Expect(err).To(MatchError(ContainSubstring("get-error")))
			Expect(objectReconciler.reconcileResourceCallCount).To(Equal(0))
		})
	})

	It("calls the object reconciler", func() {
		Expect(objectReconciler.reconcileResourceCallCount).To(Equal(1))
		Expect(objectReconciler.reconcileResourceObj.Namespace).To(Equal(pod.Namespace))
		Expect(objectReconciler.reconcileResourceObj.Name).To(Equal(pod.Name))
	})

	It("patches the object via the k8s client", func() {
		Expect(fakeClient.PatchCallCount()).To(Equal(1))
		_, updatedObject, _, _ := fakeClient.PatchArgsForCall(0)
		updatedPod, ok := updatedObject.(*corev1.Pod)
		Expect(ok).To(BeTrue())
		Expect(updatedPod.Spec.RestartPolicy).To(Equal(corev1.RestartPolicyOnFailure))
	})

	When("patching the object fails", func() {
		BeforeEach(func() {
			fakeClient.PatchReturns(errors.New("patch-object-error"))
		})

		It("returns the error", func() {
			Expect(err).To(MatchError(errors.New("patch-object-error")))
		})
	})

	It("patches the object status via the k8s client", func() {
		Expect(fakeStatusWriter.PatchCallCount()).To(Equal(1))
		_, updatedObject, _, _ := fakeStatusWriter.PatchArgsForCall(0)
		updatedPod, ok := updatedObject.(*corev1.Pod)
		Expect(ok).To(BeTrue())
		Expect(updatedPod.Status.Message).To(Equal("hello"))
	})

	When("patching the object status fails", func() {
		BeforeEach(func() {
			fakeStatusWriter.PatchReturns(errors.New("patch-status-error"))
		})

		It("returns the error", func() {
			Expect(err).To(MatchError(errors.New("patch-status-error")))
		})
	})

	It("succeeds and returns the result from the object reconciler", func() {
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(Equal(ctrl.Result{RequeueAfter: 1}))
	})

	When("the object reconciliation fails", func() {
		BeforeEach(func() {
			objectReconciler.reconcileResourceError = errors.New("reconcile-error")
		})

		It("returns the error", func() {
			Expect(err).To(MatchError("reconcile-error"))
		})

		It("updates the object and its status nevertheless", func() {
			Expect(fakeClient.PatchCallCount()).To(Equal(1))
			Expect(fakeStatusWriter.PatchCallCount()).To(Equal(1))
		})
	})

	Describe("logging", func() {
		var logOutput *gbytes.Buffer

		BeforeEach(func() {
			logOutput = gbytes.NewBuffer()
			GinkgoWriter.TeeTo(logOutput)
			logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))
		})

		It("captures logs from object reconciler", func() {
			Eventually(logOutput).Should(SatisfyAll(
				gbytes.Say("fake reconciler reconciling"),
				gbytes.Say(`"namespace":`),
				gbytes.Say(`"name":`),
				gbytes.Say(`"logID":`),
			))
		})
	})
})
