package conditions

import (
	"context"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type RuntimeObjectWithStatusConditions interface {
	client.Object
	StatusConditions() []metav1.Condition
}

type objectList[L any] interface {
	*L
	client.ObjectList
}

type Awaiter[T RuntimeObjectWithStatusConditions, L any, PL objectList[L]] struct {
	timeout time.Duration
}

func NewConditionAwaiter[T RuntimeObjectWithStatusConditions, L any, PL objectList[L]](timeout time.Duration) *Awaiter[T, L, PL] {
	return &Awaiter[T, L, PL]{
		timeout: timeout,
	}
}

func (a *Awaiter[T, L, PL]) AwaitCondition(ctx context.Context, k8sClient client.WithWatch, object client.Object, conditionType string) (T, error) {
	var empty T
	objList := PL(new(L))

	ctxWithTimeout, cancel := context.WithTimeout(ctx, a.timeout)
	defer cancel()

	watch, err := k8sClient.Watch(ctxWithTimeout,
		objList,
		client.InNamespace(object.GetNamespace()),
		client.MatchingFields{"metadata.name": object.GetName()},
	)
	if err != nil {
		return empty, err
	}
	defer watch.Stop()

	for e := range watch.ResultChan() {
		obj, ok := e.Object.(T)
		if !ok {
			continue
		}

		if meta.IsStatusConditionTrue(obj.StatusConditions(), conditionType) {
			return obj, nil
		}
	}

	return empty, fmt.Errorf("object %s:%s did not get the %s condition within timeout period %d ms",
		object.GetNamespace(), object.GetName(), conditionType, a.timeout.Milliseconds(),
	)
}
