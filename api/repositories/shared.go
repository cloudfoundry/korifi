package repositories

import (
	"context"
	"fmt"
	"maps"
	"time"

	"code.cloudfoundry.org/korifi/api/authorization"
	"github.com/BooleanCat/go-functional/v2/it/itx"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

//go:generate go run github.com/maxbrunsfeld/counterfeiter/v6 -generate

//counterfeiter:generate -o fake -fake-name RepositoryCreator . RepositoryCreator
type RepositoryCreator interface {
	CreateRepository(ctx context.Context, name string) error
}

type Awaiter[T runtime.Object] interface {
	AwaitCondition(context.Context, client.WithWatch, client.Object, string) (T, error)
	AwaitState(context.Context, client.WithWatch, client.Object, func(T) error) (T, error)
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

func authorizedSpaceNamespaces(ctx context.Context, authInfo authorization.Info, namespacePermissions *authorization.NamespacePermissions) (itx.Iterator[string], error) {
	nsList, err := namespacePermissions.GetAuthorizedSpaceNamespaces(ctx, authInfo)
	if err != nil {
		return nil, fmt.Errorf("failed to list namespaces for spaces with user role bindings: %w", err)
	}

	return itx.From(maps.Keys(nsList)), nil
}

func authorizedOrgNamespaces(ctx context.Context, authInfo authorization.Info, namespacePermissions *authorization.NamespacePermissions) (itx.Iterator[string], error) {
	nsList, err := namespacePermissions.GetAuthorizedOrgNamespaces(ctx, authInfo)
	if err != nil {
		return nil, fmt.Errorf("failed to list namespaces for orgs with user role bindings: %w", err)
	}

	return itx.From(maps.Keys(nsList)), nil
}
