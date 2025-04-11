package conditions

import (
	"context"
	"fmt"
	"time"

	"code.cloudfoundry.org/korifi/api/repositories"
	"code.cloudfoundry.org/korifi/tools/k8s/conditions"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type ObjectList[L any] interface {
	*L
	client.ObjectList
}

type Awaiter[T conditions.RuntimeObjectWithStatusConditions, L any, PL ObjectList[L]] struct {
	timeout time.Duration
}

func NewConditionAwaiter[T conditions.RuntimeObjectWithStatusConditions, L any, PL ObjectList[L]](timeout time.Duration) *Awaiter[T, L, PL] {
	return &Awaiter[T, L, PL]{
		timeout: timeout,
	}
}

func (a *Awaiter[T, L, PL]) AwaitCondition(ctx context.Context, k8sClient repositories.Klient, object client.Object, conditionType string) (T, error) {
	return a.AwaitState(ctx, k8sClient, object, func(o T) error {
		return conditions.CheckConditionIsTrue[T](o, conditionType)
	})
}

func (a *Awaiter[T, L, PL]) AwaitState(ctx context.Context, k8sClient repositories.Klient, object client.Object, checkState func(T) error) (T, error) {
	var empty T
	objList := PL(new(L))

	ctxWithTimeout, cancel := context.WithTimeout(ctx, a.timeout)
	defer cancel()

	watch, err := k8sClient.Watch(ctxWithTimeout,
		objList,
		repositories.InNamespace(object.GetNamespace()),
		repositories.MatchingFields{"metadata.name": object.GetName()},
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
