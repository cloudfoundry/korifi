package repositories

import (
	"context"
	"fmt"
	"maps"
	"time"

	"code.cloudfoundry.org/korifi/api/authorization"
	"github.com/BooleanCat/go-functional/v2/it/itx"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
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

type Awaiter[T runtime.Object] interface {
	AwaitCondition(context.Context, Klient, client.Object, string) (T, error)
	AwaitState(context.Context, Klient, client.Object, func(T) error) (T, error)
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
