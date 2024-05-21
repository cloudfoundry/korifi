package fakeawaiter

import (
	"context"

	"code.cloudfoundry.org/korifi/api/repositories/conditions"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type FakeAwaiter[T conditions.RuntimeObjectWithStatusConditions, L any, PL conditions.ObjectList[L]] struct {
	awaitConditionCalls []struct {
		obj           client.Object
		conditionType string
	}
	AwaitConditionStub func(context.Context, client.WithWatch, client.Object, string) (T, error)
	AwaitStateStub     func(context.Context, client.WithWatch, client.Object, func(T) error) (T, error)
	awaitStateCalls    []struct {
		obj        client.Object
		checkState func(T) error
	}
}

func (a *FakeAwaiter[T, L, PL]) AwaitCondition(ctx context.Context, k8sClient client.WithWatch, object client.Object, conditionType string) (T, error) {
	a.awaitConditionCalls = append(a.awaitConditionCalls, struct {
		obj           client.Object
		conditionType string
	}{
		object,
		conditionType,
	})

	if a.AwaitConditionStub == nil {
		return object.(T), nil
	}

	return a.AwaitConditionStub(ctx, k8sClient, object, conditionType)
}

func (a *FakeAwaiter[T, L, PL]) AwaitConditionReturns(object T, err error) {
	a.AwaitConditionStub = func(ctx context.Context, k8sClient client.WithWatch, object client.Object, conditionType string) (T, error) {
		return object.(T), err
	}
}

func (a *FakeAwaiter[T, L, PL]) AwaitConditionCallCount() int {
	return len(a.awaitConditionCalls)
}

func (a *FakeAwaiter[T, L, PL]) AwaitConditionArgsForCall(i int) (client.Object, string) {
	return a.awaitConditionCalls[i].obj, a.awaitConditionCalls[i].conditionType
}

func (a *FakeAwaiter[T, L, PL]) AwaitState(ctx context.Context, k8sClient client.WithWatch, object client.Object, checkState func(T) error) (T, error) {
	a.awaitStateCalls = append(a.awaitStateCalls, struct {
		obj        client.Object
		checkState func(T) error
	}{
		object,
		checkState,
	})

	if a.AwaitStateStub == nil {
		return object.(T), nil
	}

	return a.AwaitStateStub(ctx, k8sClient, object, checkState)
}

func (a *FakeAwaiter[T, L, PL]) AwaitStateReturns(object T, err error) {
	a.AwaitStateStub = func(ctx context.Context, k8sClient client.WithWatch, object client.Object, _ func(T) error) (T, error) {
		return object.(T), err
	}
}

func (a *FakeAwaiter[T, L, PL]) AwaitStateCallCount() int {
	return len(a.awaitStateCalls)
}

func (a *FakeAwaiter[T, L, PL]) AwaitStateArgsForCall(i int) (client.Object, func(T) error) {
	return a.awaitStateCalls[i].obj, a.awaitStateCalls[i].checkState
}
