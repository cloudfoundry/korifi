package k8s_test

import (
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/tools/k8s"
)

var _ = Describe("ReadyConditionBuilder", func() {
	var (
		builder   *k8s.ReadyConditionBuilder
		condition metav1.Condition
	)

	BeforeEach(func() {
		builder = k8s.NewReadyConditionBuilder(&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Generation: 4,
			},
		})
	})

	JustBeforeEach(func() {
		condition = builder.Build()
	})

	It("creates a default condition", func() {
		Expect(condition.Type).To(Equal(korifiv1alpha1.StatusConditionReady))
		Expect(condition.Status).To(Equal(metav1.ConditionFalse))
		Expect(condition.ObservedGeneration).To(BeEquivalentTo(4))
		Expect(condition.LastTransitionTime).NotTo(BeZero())
		Expect(condition.Reason).To(Equal("Unknown"))
		Expect(condition.Message).To(BeEmpty())
	})

	When("the status is set to true", func() {
		BeforeEach(func() {
			builder.WithStatus(metav1.ConditionTrue)
		})

		It("sets condition status to true", func() {
			Expect(condition.Status).To(Equal(metav1.ConditionTrue))
		})
	})

	When("the reason is set", func() {
		BeforeEach(func() {
			builder.WithReason("SomeReason")
		})

		It("sets the reason", func() {
			Expect(condition.Reason).To(Equal("SomeReason"))
		})
	})

	When("the message is set", func() {
		BeforeEach(func() {
			builder.WithMessage("Some Message")
		})

		It("sets the reason", func() {
			Expect(condition.Message).To(Equal("Some Message"))
		})
	})

	When("the error is set", func() {
		BeforeEach(func() {
			builder.WithError(errors.New("some-error"))
		})

		It("sets the reason", func() {
			Expect(condition.Message).To(Equal("some-error"))
		})
	})

	When("nil error is set", func() {
		BeforeEach(func() {
			builder.WithError(nil)
		})

		It("sets the reason", func() {
			Expect(condition.Message).To(BeEmpty())
		})
	})

	When("ready is set", func() {
		BeforeEach(func() {
			builder.Ready()
		})

		It("sets ready status and reason", func() {
			Expect(condition.Status).To(Equal(metav1.ConditionTrue))
			Expect(condition.Reason).To(Equal("Ready"))
			Expect(condition.Message).To(Equal("Ready"))
		})

		When("error is set", func() {
			BeforeEach(func() {
				builder.WithError(errors.New("foo"))
			})

			It("ignores the error and returns ready condition", func() {
				Expect(condition.Status).To(Equal(metav1.ConditionTrue))
				Expect(condition.Reason).To(Equal("Ready"))
				Expect(condition.Message).To(Equal("Ready"))
			})
		})
	})
})
