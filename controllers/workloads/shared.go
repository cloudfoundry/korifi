package workloads

import (
	"context"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

//go:generate go run github.com/maxbrunsfeld/counterfeiter/v6 -o fake -fake-name CFClient . CFClient
type CFClient interface {
	Get(ctx context.Context, key client.ObjectKey, obj client.Object) error
	Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error
	Status() client.StatusWriter
}

// This is a helper function for updating local copy of status conditions
func setStatusConditionOnLocalCopy(conditions *[]metav1.Condition, conditionType string, conditionStatus metav1.ConditionStatus, reason, message string) {
	meta.SetStatusCondition(conditions, metav1.Condition{
		Type:    conditionType,
		Status:  conditionStatus,
		Reason:  reason,
		Message: message,
	})
}
