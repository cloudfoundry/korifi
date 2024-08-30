package conditions

import (
	"context"
	"fmt"
	"time"

	"code.cloudfoundry.org/korifi/tools/k8s"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type ObjectList[L any] interface {
	*L
	client.ObjectList
}

type Awaiter[T k8s.RuntimeObjectWithStatusConditions[TT], TT any, L any, PL ObjectList[L]] struct {
	timeout time.Duration
}

func NewConditionAwaiter[T k8s.RuntimeObjectWithStatusConditions[TT], TT any, L any, PL ObjectList[L]](timeout time.Duration) *Awaiter[T, TT, L, PL] {
	return &Awaiter[T, TT, L, PL]{
		timeout: timeout,
	}
}

func (a *Awaiter[T, TT, L, PL]) AwaitCondition(ctx context.Context, k8sClient client.WithWatch, object client.Object, conditionType string) (T, error) {
	return a.AwaitState(ctx, k8sClient, object, func(o T) error {
		return checkConditionIsTrue(o, conditionType)
	})
}

func (a *Awaiter[T, TT, L, PL]) AwaitState(ctx context.Context, k8sClient client.WithWatch, object client.Object, checkState func(T) error) (T, error) {
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

	var checkStateErr error
	for e := range watch.ResultChan() {
		obj, ok := e.Object.(T)
		if !ok {
			continue
		}

		checkStateErr = checkState(obj)
		if checkStateErr == nil {
			return obj, nil
		}
	}

	return empty, fmt.Errorf("object %s/%s state has not been met in %.2f s: %s",
		object.GetNamespace(), object.GetName(), a.timeout.Seconds(), checkStateErr.Error(),
	)
}

func checkConditionIsTrue[T k8s.RuntimeObjectWithStatusConditions[L], L any](obj T, conditionType string) error {
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
	return fmt.Errorf("expected the %s condition to be true", conditionType)
}
