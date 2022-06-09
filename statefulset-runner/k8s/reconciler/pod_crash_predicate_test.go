package reconciler_test

import (
	"code.cloudfoundry.org/korifi/statefulset-runner/k8s/reconciler"
	"code.cloudfoundry.org/korifi/statefulset-runner/k8s/stset"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/event"
)

var _ = Describe("PodCrashPredicate", func() {
	var predicate reconciler.SourceTypeUpdatePredicate

	BeforeEach(func() {
		predicate = reconciler.NewSourceTypeUpdatePredicate("very-wow-much-awesome")
	})

	It("rejects all Create/Delete/Generic calls", func() {
		Expect(predicate.Create(event.CreateEvent{})).To(BeFalse())
		Expect(predicate.Delete(event.DeleteEvent{})).To(BeFalse())
		Expect(predicate.Generic(event.GenericEvent{})).To(BeFalse())
	})

	It("rejects all Update calls without specified source type", func() {
		p := corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Labels: map[string]string{
					stset.LabelSourceType: "sic-mundus",
				},
			},
		}
		Expect(predicate.Update(event.UpdateEvent{ObjectNew: &p})).To(BeFalse())
	})

	It("allows all Update calls with specified source type", func() {
		p := corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Labels: map[string]string{
					stset.LabelSourceType: "very-wow-much-awesome",
				},
			},
		}
		Expect(predicate.Update(event.UpdateEvent{ObjectNew: &p})).To(BeTrue())
	})
})
