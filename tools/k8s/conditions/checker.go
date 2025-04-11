package conditions

import (
	"fmt"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type RuntimeObjectWithStatusConditions interface {
	client.Object
	StatusConditions() *[]metav1.Condition
}

func CheckConditionIsTrue[T RuntimeObjectWithStatusConditions](obj T, conditionType string) error {
	condition := meta.FindStatusCondition(*obj.StatusConditions(), conditionType)

	if condition == nil {
		return fmt.Errorf("condition %s not set yet", conditionType)
	}

	if condition.ObservedGeneration != obj.GetGeneration() {
		return fmt.Errorf("condition %s is outdated", conditionType)
	}

	if condition.Status == metav1.ConditionTrue {
		return nil
	}
	return fmt.Errorf("%s condition is not true", conditionType)
}
