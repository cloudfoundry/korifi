package conditions_test

import (
	"context"
	"time"

	"code.cloudfoundry.org/korifi/api/repositories/conditions"
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("Set Conditions", func() {
	var (
		space      *korifiv1alpha1.CFSpace
		setCondErr error
		ctx        context.Context
	)

	initStatus := func(space *korifiv1alpha1.CFSpace) error {
		originalSpace := space.DeepCopy()
		space.Status.GUID = "space-123"
		space.Status.Conditions = []metav1.Condition{}
		return k8sClient.Status().Patch(context.Background(), space, client.MergeFrom(originalSpace))
	}

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
		Expect(initStatus(space)).To(Succeed())
	})

	JustBeforeEach(func() {
		setCondErr = conditions.PatchStatus(ctx, k8sClient, space, metav1.Condition{
			Type:    "Ready",
			Status:  metav1.ConditionTrue,
			Reason:  "whatevs",
			Message: "whatevs",
		})
	})

	It("mutates the original object conditions", func() {
		Expect(setCondErr).NotTo(HaveOccurred())
		Expect(meta.IsStatusConditionTrue(space.Status.Conditions, "Ready")).To(BeTrue())
	})

	It("updates the object in K8S", func() {
		Expect(setCondErr).NotTo(HaveOccurred())
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
			Expect(setCondErr).To(MatchError(ContainSubstring("deadline exceeded")))
		})
	})
})
