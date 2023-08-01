package repositories

import (
	"context"
	"time"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	StatusConditionReady                 = "Ready"
	VCAPServicesSecretAvailableCondition = "VCAPServicesSecretAvailable"
)

//go:generate go run github.com/maxbrunsfeld/counterfeiter/v6 -generate

//counterfeiter:generate -o fake -fake-name RepositoryCreator . RepositoryCreator
type RepositoryCreator interface {
	CreateRepository(ctx context.Context, name string) error
}

type ConditionAwaiter[T runtime.Object] interface {
	AwaitCondition(ctx context.Context, userClient client.WithWatch, object client.Object, conditionType string) (T, error)
}

func getLastUpdatedTime(obj client.Object) *time.Time {
	managedFields := obj.GetManagedFields()
	if len(managedFields) == 0 {
		return nil
	}

	var latestTime *metav1.Time
	for _, managedField := range managedFields {
		currentTime := managedField.Time
		if latestTime == nil {
			latestTime = currentTime
		} else if currentTime != nil {
			if currentTime.After(latestTime.Time) {
				latestTime = currentTime
			}
		}
	}
	return golangTime(latestTime)
}

func golangTime(t *metav1.Time) *time.Time {
	if t == nil {
		return nil
	}
	return &t.Time
}

// getConditionValue is a helper function that retrieves the value of the provided conditionType, like "Succeeded" and returns the value: "True", "False", or "Unknown"
// If the value is not present, returns Unknown
func getConditionValue(conditions *[]metav1.Condition, conditionType string) metav1.ConditionStatus {
	conditionStatusValue := metav1.ConditionUnknown
	conditionStatus := meta.FindStatusCondition(*conditions, conditionType)
	if conditionStatus != nil {
		conditionStatusValue = conditionStatus.Status
	}
	return conditionStatusValue
}

func getLabelOrAnnotation(mapObj map[string]string, key string) string {
	if mapObj == nil {
		return ""
	}
	return mapObj[key]
}

type Set[T comparable] map[T]struct{}

func (s Set[T]) Includes(element T) bool {
	_, ok := s[element]
	return ok
}

func NewSet[T comparable](elements ...T) Set[T] {
	set := Set[T]{}
	for _, e := range elements {
		set[e] = struct{}{}
	}
	return set
}

func Filter[T any](resources []T, predicate ...func(T) bool) []T {
	var res []T
outer:
	for _, r := range resources {
		for _, p := range predicate {
			if !p(r) {
				continue outer
			}
		}
		res = append(res, r)
	}

	return res
}

func SetPredicate[T comparable, S any](elements []T, mapFn func(S) T) func(S) bool {
	if len(elements) == 0 {
		return AlwaysTrue[S]
	}

	set := NewSet(elements...)
	return func(e S) bool {
		return set.Includes(mapFn(e))
	}
}

func AlwaysTrue[T any](_ T) bool { return true }
