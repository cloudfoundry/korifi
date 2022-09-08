package k8s_test

import (
	"context"
	"time"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/tools/k8s"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("Kubernetes Status", func() {
	var (
		ctx      context.Context
		space    *korifiv1alpha1.CFSpace
		patchErr error
	)

	BeforeEach(func() {
		ctx = context.Background()
		space = &korifiv1alpha1.CFSpace{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace,
				Name:      "my-space",
			},
			Spec: korifiv1alpha1.CFSpaceSpec{
				DisplayName: "foo",
			},
		}

		Expect(k8sClient.Create(context.Background(), space)).To(Succeed())
	})

	Describe("PatchStatusConditions", func() {
		JustBeforeEach(func() {
			patchErr = k8s.PatchStatusConditions(ctx, k8sClient, space, metav1.Condition{
				Type:    "Ready",
				Status:  metav1.ConditionTrue,
				Reason:  "whatevs",
				Message: "whatevs",
			})
		})

		It("mutates the original object conditions", func() {
			Expect(patchErr).NotTo(HaveOccurred())
			Expect(meta.IsStatusConditionTrue(space.Status.Conditions, "Ready")).To(BeTrue())
		})

		It("updates the object in K8S", func() {
			Expect(patchErr).NotTo(HaveOccurred())
			updatedSpace := &korifiv1alpha1.CFSpace{}
			Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(space), updatedSpace)).To(Succeed())
			Expect(meta.IsStatusConditionTrue(updatedSpace.Status.Conditions, "Ready")).To(BeTrue())
		})

		When("patching the status fails", func() {
			var cancel context.CancelFunc
			BeforeEach(func() {
				ctx, cancel = context.WithDeadline(ctx, time.Now().Add(-1*time.Minute))
			})

			AfterEach(func() {
				cancel()
			})

			It("returns an error", func() {
				Expect(patchErr).To(MatchError(ContainSubstring("deadline exceeded")))
			})
		})
	})

	Describe("PatchStatus", func() {
		JustBeforeEach(func() {
			patchErr = k8s.PatchStatus(ctx, k8sClient, space, func() {
				space.Status.GUID = "foo"
			}, metav1.Condition{
				Type:    "Ready",
				Status:  metav1.ConditionTrue,
				Reason:  "whatevs",
				Message: "whatevs",
			})
		})

		It("mutates the original object conditions", func() {
			Expect(patchErr).NotTo(HaveOccurred())
			Expect(space.Status.GUID).To(Equal("foo"))
			Expect(meta.IsStatusConditionTrue(space.Status.Conditions, "Ready")).To(BeTrue())
		})

		It("updates the object in K8S", func() {
			Expect(patchErr).NotTo(HaveOccurred())
			updatedSpace := &korifiv1alpha1.CFSpace{}
			Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(space), updatedSpace)).To(Succeed())
			Expect(updatedSpace.Status.GUID).To(Equal("foo"))
			Expect(meta.IsStatusConditionTrue(updatedSpace.Status.Conditions, "Ready")).To(BeTrue())
		})

		When("patching the status fails", func() {
			var cancel context.CancelFunc
			BeforeEach(func() {
				ctx, cancel = context.WithDeadline(ctx, time.Now().Add(-1*time.Minute))
			})

			AfterEach(func() {
				cancel()
			})

			It("returns an error", func() {
				Expect(patchErr).To(MatchError(ContainSubstring("deadline exceeded")))
			})
		})
	})
})
