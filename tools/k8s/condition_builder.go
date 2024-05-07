package k8s

import (
	"time"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type ReadyConditionBuilder struct {
	status             metav1.ConditionStatus
	observedGeneration int64
	reason             string
	message            string
}

func NewReadyConditionBuilder(obj client.Object) *ReadyConditionBuilder {
	return &ReadyConditionBuilder{
		status:             metav1.ConditionFalse,
		observedGeneration: obj.GetGeneration(),
		reason:             "Unknown",
	}
}

func (b *ReadyConditionBuilder) WithStatus(status metav1.ConditionStatus) *ReadyConditionBuilder {
	b.status = status
	return b
}

func (b *ReadyConditionBuilder) WithReason(reason string) *ReadyConditionBuilder {
	b.reason = reason
	return b
}

func (b *ReadyConditionBuilder) WithMessage(message string) *ReadyConditionBuilder {
	b.message = message
	return b
}

func (b *ReadyConditionBuilder) WithError(err error) *ReadyConditionBuilder {
	if err == nil {
		return b
	}

	b.message = err.Error()
	return b
}

func (b *ReadyConditionBuilder) Ready() *ReadyConditionBuilder {
	return b.WithStatus(metav1.ConditionTrue)
}

func (b *ReadyConditionBuilder) Build() metav1.Condition {
	result := metav1.Condition{
		Type:               korifiv1alpha1.StatusConditionReady,
		Status:             b.status,
		ObservedGeneration: b.observedGeneration,
		LastTransitionTime: metav1.NewTime(time.Now()),
		Reason:             b.reason,
		Message:            b.message,
	}

	if b.status == metav1.ConditionTrue {
		result.Reason = "Ready"
		result.Message = "Ready"
	}

	return result
}
