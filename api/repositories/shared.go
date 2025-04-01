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
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

//go:generate go run github.com/maxbrunsfeld/counterfeiter/v6 -generate

type ResourceState int

const (
	ResourceStateUnknown ResourceState = iota
	ResourceStateReady
)

//counterfeiter:generate -o fake -fake-name RepositoryCreator . RepositoryCreator
type RepositoryCreator interface {
	CreateRepository(ctx context.Context, name string) error
}

//counterfeiter:generate -o fake -fake-name Klient . Klient
type Klient interface {
	Get(ctx context.Context, obj client.Object) error
	Create(ctx context.Context, obj client.Object) error
	Patch(ctx context.Context, obj client.Object, modify func() error) error
	List(ctx context.Context, list client.ObjectList, opts ...ListOption) error
	Watch(ctx context.Context, obj client.ObjectList, opts ...ListOption) (watch.Interface, error)
	Delete(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error
}

type ListOptions struct {
	SpaceGUIDs    []string
	LabelSelector labels.Selector
	Namespace     string
	FieldSelector fields.Selector
}

type ListOption interface {
	ApplyToList(*ListOptions)
}

type WithSpaceGUIDs []string

func (o WithSpaceGUIDs) ApplyToList(opts *ListOptions) {
	opts.SpaceGUIDs = []string(o)
}

type InNamespace string

func (o InNamespace) ApplyToList(opts *ListOptions) {
	opts.Namespace = string(o)
}

type WithLabels struct {
	labels.Selector
}

func (o WithLabels) ApplyToList(opts *ListOptions) {
	opts.LabelSelector = labels.Selector(o)
}

type MatchingFields fields.Set

func (m MatchingFields) ApplyToList(opts *ListOptions) {
	sel := fields.Set(m).AsSelector()
	opts.FieldSelector = sel
}

type Watcher interface {
	Watch(ctx context.Context, obj client.ObjectList, opts ...ListOption) (watch.Interface, error)
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
