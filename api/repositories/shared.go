package repositories

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	"code.cloudfoundry.org/korifi/api/authorization"
	"code.cloudfoundry.org/korifi/api/repositories/k8sklient/descriptors"
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
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

type ListResult[T any] struct {
	PageInfo descriptors.PageInfo
	Records  []T
}

type Pagination struct {
	PerPage int
	Page    int
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

func getAuthorizedSpaceNamespaces(ctx context.Context, authInfo authorization.Info, namespacePermissions *authorization.NamespacePermissions) ([]string, error) {
	nsList, err := namespacePermissions.GetAuthorizedSpaceNamespaces(ctx, authInfo)
	if err != nil {
		return nil, fmt.Errorf("failed to list namespaces for spaces with user role bindings: %w", err)
	}

	return slices.Collect(maps.Keys(nsList)), nil
}

func getAuthorizedOrgNamespaces(ctx context.Context, authInfo authorization.Info, namespacePermissions *authorization.NamespacePermissions) ([]string, error) {
	nsList, err := namespacePermissions.GetAuthorizedOrgNamespaces(ctx, authInfo)
	if err != nil {
		return nil, fmt.Errorf("failed to list namespaces for orgs with user role bindings: %w", err)
	}

	return slices.Collect(maps.Keys(nsList)), nil
}

func getAuthorizedNamespaces(ctx context.Context, authInfo authorization.Info, namespacePermissions *authorization.NamespacePermissions) ([]string, error) {
	authorizedOrgNamespaces, err := getAuthorizedOrgNamespaces(ctx, authInfo, namespacePermissions)
	if err != nil {
		return nil, err
	}

	authorizedSpaceNamespaces, err := getAuthorizedSpaceNamespaces(ctx, authInfo, namespacePermissions)
	if err != nil {
		return nil, err
	}

	return append(authorizedOrgNamespaces, authorizedSpaceNamespaces...), nil
}

func getCreatedUpdatedAt(obj metav1.Object) (time.Time, *time.Time, error) {
	createdAt, err := time.Parse(korifiv1alpha1.LabelDateFormat, obj.GetLabels()[korifiv1alpha1.CreatedAtLabelKey])
	if err != nil {
		return time.Time{}, nil, fmt.Errorf("failed to parse %q label: %w", korifiv1alpha1.CreatedAtLabelKey, err)
	}

	var updatedAt *time.Time
	if obj.GetLabels()[korifiv1alpha1.UpdatedAtLabelKey] != "" {
		updateTime, err := time.Parse(korifiv1alpha1.LabelDateFormat, getLabelOrAnnotation(obj.GetLabels(), korifiv1alpha1.UpdatedAtLabelKey))
		if err != nil {
			return time.Time{}, nil, fmt.Errorf("failed to parse %q label: %w", korifiv1alpha1.UpdatedAtLabelKey, err)
		}
		updatedAt = &updateTime
	}

	return createdAt, updatedAt, nil
}

func toCustomSortOption(requestedOrderBy string, orderByToColumn map[string]string) ListOption {
	desc := false
	orderBy := requestedOrderBy
	if strings.HasPrefix(requestedOrderBy, "-") {
		desc = true
		orderBy = strings.TrimPrefix(requestedOrderBy, "-")
	}

	if orderBy == "" {
		return NoopListOption{}
	}

	column, ok := orderByToColumn[orderBy]
	if !ok {
		return ErroringListOption(fmt.Sprintf("unsupported field for ordering: %q", orderBy))
	}

	return SortBy(column, desc)
}

func toSortOption(requestedOrderBy string) ListOption {
	defaultOrderByToColumn := map[string]string{
		"created_at": "Created At",
		"updated_at": "Updated At",
		"name":       "Display Name",
		"state":      "State",
	}

	return toCustomSortOption(requestedOrderBy, defaultOrderByToColumn)
}
