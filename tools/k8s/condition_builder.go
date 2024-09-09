package k8s

import (
	"fmt"
	"time"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/tools"
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
	if message != "" {
		b.message = message
	}
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

type NotReadyError struct {
	cause        error
	reason       string
	message      string
	requeueAfter *time.Duration
	requeue      bool
	noRequeue    bool
}

func (e NotReadyError) Error() string {
	if e.cause != nil {
		return fmt.Sprintf("%s: %s", e.message, e.cause.Error())
	}
	return e.message
}

func NewNotReadyError() NotReadyError {
	return NotReadyError{}
}

func (e NotReadyError) WithCause(cause error) NotReadyError {
	e.cause = cause
	return e
}

func (e NotReadyError) WithRequeue() NotReadyError {
	e.requeue = true
	return e
}

func (e NotReadyError) WithRequeueAfter(duration time.Duration) NotReadyError {
	e.requeueAfter = tools.PtrTo(duration)
	return e
}

func (e NotReadyError) WithNoRequeue() NotReadyError {
	e.noRequeue = true
	return e
}

func (e NotReadyError) WithReason(reason string) NotReadyError {
	e.reason = reason
	return e
}

func (e NotReadyError) WithMessage(message string) NotReadyError {
	e.message = message
	return e
}
