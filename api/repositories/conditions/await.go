package conditions

import (
	"context"
	"fmt"
	"time"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type awaitConfig interface {
	watchObjectList() client.ObjectList
	conditions() func(runtime.Object) ([]metav1.Condition, bool)
}

type cfTaskAwaitConfig struct{}

func (c cfTaskAwaitConfig) watchObjectList() client.ObjectList {
	return &korifiv1alpha1.CFTaskList{}
}

func (c cfTaskAwaitConfig) conditions() func(runtime.Object) ([]metav1.Condition, bool) {
	return func(obj runtime.Object) ([]metav1.Condition, bool) {
		cfTask, ok := obj.(*korifiv1alpha1.CFTask)
		if !ok {
			return nil, false
		}

		return cfTask.Status.Conditions, true
	}
}

type cfAppAwaitConfig struct{}

func (c cfAppAwaitConfig) watchObjectList() client.ObjectList {
	return &korifiv1alpha1.CFAppList{}
}

func (c cfAppAwaitConfig) conditions() func(runtime.Object) ([]metav1.Condition, bool) {
	return func(obj runtime.Object) ([]metav1.Condition, bool) {
		cfApp, ok := obj.(*korifiv1alpha1.CFApp)
		if !ok {
			return nil, false
		}

		return cfApp.Status.Conditions, true
	}
}

type Awaiter struct {
	timeout time.Duration
	config  awaitConfig
}

func NewCFTaskConditionAwaiter(timeout time.Duration) *Awaiter {
	return &Awaiter{
		timeout: timeout,
		config:  cfTaskAwaitConfig{},
	}
}

func NewCFAppConditionAwaiter(timeout time.Duration) *Awaiter {
	return &Awaiter{
		timeout: timeout,
		config:  cfAppAwaitConfig{},
	}
}

func (a *Awaiter) AwaitCondition(ctx context.Context, k8sClient client.WithWatch, object client.Object, conditionType string) (runtime.Object, error) {
	watch, err := k8sClient.Watch(ctx,
		a.config.watchObjectList(),
		client.InNamespace(object.GetNamespace()),
		client.MatchingFields{"metadata.name": object.GetName()},
	)
	if err != nil {
		return nil, err
	}
	defer watch.Stop()

	timer := time.NewTimer(a.timeout)
	defer timer.Stop()

	for {
		select {
		case e := <-watch.ResultChan():
			conditions, ok := a.config.conditions()(e.Object)
			if !ok {
				continue
			}

			if meta.IsStatusConditionTrue(conditions, conditionType) {
				return e.Object, nil
			}
		case <-timer.C:
			return nil, fmt.Errorf(
				"object %s:%s did not get the %s condition within timeout period %d ms",
				object.GetNamespace(), object.GetName(), conditionType, a.timeout.Milliseconds(),
			)
		}
	}
}
