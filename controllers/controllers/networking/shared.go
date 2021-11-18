package networking

import (
	"time"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

//go:generate go run github.com/maxbrunsfeld/counterfeiter/v6 -generate

//counterfeiter:generate -o fake -fake-name Client sigs.k8s.io/controller-runtime/pkg/client.Client

//counterfeiter:generate -o fake -fake-name StatusWriter sigs.k8s.io/controller-runtime/pkg/client.StatusWriter
// This is a helper function for updating local copy of status conditions

func setStatusConditionOnLocalCopy(conditions *[]metav1.Condition, conditionType string, conditionStatus metav1.ConditionStatus, reason, message string) {
	meta.SetStatusCondition(conditions, metav1.Condition{
		Type:   conditionType,
		Status: conditionStatus,
		LastTransitionTime: metav1.Time{
			Time: time.Now(),
		},
		Reason:  reason,
		Message: message,
	})
}
