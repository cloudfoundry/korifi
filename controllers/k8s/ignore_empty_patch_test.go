package k8s_test

import (
	"code.cloudfoundry.org/korifi/controllers/k8s"
	"code.cloudfoundry.org/korifi/controllers/k8s/fake"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

//go:generate go run github.com/maxbrunsfeld/counterfeiter/v6 -generate
//counterfeiter:generate -o fake -fake-name StatusWriter sigs.k8s.io/controller-runtime/pkg/client.StatusWriter
//counterfeiter:generate -o fake -fake-name Client sigs.k8s.io/controller-runtime/pkg/client.Client

var _ = Describe("IgnoreEmptyPatches", func() {
	var (
		fakeClient *fake.Client
		k8sClient  *k8s.IgnoreEmptyPatchesClient

		pod         *corev1.Pod
		originalPod *corev1.Pod
		patchErr    error
	)

	BeforeEach(func() {
		fakeClient = new(fake.Client)

		k8sClient = k8s.IgnoreEmptyPatches(fakeClient)

		pod = &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: uuid.NewString(),
				Name:      uuid.NewString(),
			},
		}
		originalPod = pod.DeepCopy()
	})

	Describe("Patch", func() {
		JustBeforeEach(func() {
			patchErr = k8sClient.Patch(ctx, pod, client.MergeFrom(originalPod), client.DryRunAll)
		})

		It("does not perform the patch on the server", func() {
			Expect(patchErr).NotTo(HaveOccurred())
			Expect(fakeClient.PatchCallCount()).To(BeZero())
		})

		When("the object has been updated", func() {
			BeforeEach(func() {
				pod.Spec.RestartPolicy = corev1.RestartPolicyNever
			})

			It("performs the patch  on the server", func() {
				Expect(patchErr).NotTo(HaveOccurred())
				Expect(fakeClient.PatchCallCount()).To(Equal(1))
				actualCtx, actualObject, actualPatch, actualOpts := fakeClient.PatchArgsForCall(0)
				Expect(actualCtx).To(Equal(ctx))
				Expect(actualObject).To(Equal(pod))
				Expect(actualOpts).To(ConsistOf(client.DryRunAll))

				patchContent, err := actualPatch.Data(pod)
				Expect(err).NotTo(HaveOccurred())
				Expect(string(patchContent)).To(Equal(`{"spec":{"restartPolicy":"Never"}}`))
			})
		})
	})

	Describe("StatusPatch", func() {
		var fakeStatusWriter *fake.StatusWriter

		BeforeEach(func() {
			fakeStatusWriter = new(fake.StatusWriter)
			fakeClient.StatusReturns(fakeStatusWriter)
		})

		JustBeforeEach(func() {
			patchErr = k8sClient.Status().Patch(ctx, pod, client.MergeFrom(originalPod), client.DryRunAll)
		})

		It("does not perform the patch on the server", func() {
			Expect(patchErr).NotTo(HaveOccurred())
			Expect(fakeStatusWriter.PatchCallCount()).To(BeZero())
		})

		When("the object has been updated", func() {
			BeforeEach(func() {
				pod.Status.Phase = corev1.PodSucceeded
			})

			It("performs the patch  on the server", func() {
				Expect(patchErr).NotTo(HaveOccurred())
				Expect(fakeStatusWriter.PatchCallCount()).To(Equal(1))
				actualCtx, actualObject, actualPatch, actualOpts := fakeStatusWriter.PatchArgsForCall(0)
				Expect(actualCtx).To(Equal(ctx))
				Expect(actualObject).To(Equal(pod))
				Expect(actualOpts).To(ConsistOf(client.DryRunAll))

				patchContent, err := actualPatch.Data(pod)
				Expect(err).NotTo(HaveOccurred())
				Expect(string(patchContent)).To(Equal(`{"status":{"phase":"Succeeded"}}`))
			})
		})
	})
})
