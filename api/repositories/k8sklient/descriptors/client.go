package descriptors

import (
	"context"
	"fmt"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	restclient "k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

//counterfeiter:generate -o fake -fake-name ResultSetDescriptor . ResultSetDescriptor
type ResultSetDescriptor interface {
	GUIDs() ([]string, error)
	Sort(column string, desc bool) error
	Filter(columnName string, filterFunc func(value any) bool) error
}

type Pluralizer interface {
	Pluralize(resourceGVK schema.GroupVersionKind) (string, error)
}

type FilteringOpts interface {
	Apply(ctx context.Context, opts ...client.ListOption) (*client.ListOptions, error)
}

type Client struct {
	restClient    restclient.Interface
	pluralizer    Pluralizer
	filteringOpts FilteringOpts
	scheme        *runtime.Scheme
}

func NewClient(restClient restclient.Interface, pluralizer Pluralizer, scheme *runtime.Scheme, filteringOpts FilteringOpts) *Client {
	return &Client{
		restClient:    restClient,
		pluralizer:    pluralizer,
		scheme:        scheme,
		filteringOpts: filteringOpts,
	}
}

func (c *Client) List(ctx context.Context, listGVK schema.GroupVersionKind, opts ...client.ListOption) (ResultSetDescriptor, error) {
	listOpts, err := c.filteringOpts.Apply(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to apply space filtering options: %w", err)
	}

	pluralObjectKind, err := c.pluralizer.Pluralize(toResourceGVK(listGVK))
	if err != nil {
		return nil, fmt.Errorf("failed to pluralize %s: %w", listGVK.Kind, err)
	}

	requestPath := fmt.Sprintf("/apis/%s/%s/%s",
		listGVK.Group,
		listGVK.Version,
		pluralObjectKind,
	)

	parameterCodec := runtime.NewParameterCodec(c.scheme)
	table := &metav1.Table{}
	err = c.restClient.Get().
		AbsPath(requestPath).
		SetHeader("Accept", "application/json;as=Table;g=meta.k8s.io;v=v1").
		VersionedParams(&metav1.TableOptions{
			IncludeObject: metav1.IncludeNone,
		}, parameterCodec).
		VersionedParams(listOpts.AsListOptions(), parameterCodec).
		Do(ctx).Into(table)
	if err != nil {
		return nil, fmt.Errorf("failed to list %s/%s/%s descriptors: %w", listGVK.Group, listGVK.Version, pluralObjectKind, err)
	}

	return &TableResultSetDescriptor{Table: table}, nil
}

func toResourceGVK(listGVK schema.GroupVersionKind) schema.GroupVersionKind {
	return schema.GroupVersionKind{
		Group:   listGVK.Group,
		Version: listGVK.Version,
		Kind:    strings.TrimSuffix(listGVK.Kind, "List"),
	}
}
