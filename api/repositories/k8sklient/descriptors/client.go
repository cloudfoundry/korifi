package descriptors

import (
	"context"
	"fmt"
	"strings"

	"code.cloudfoundry.org/korifi/api/authorization"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/scheme"
	restclient "k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

//counterfeiter:generate -o fake -fake-name ResultSetDescriptor . ResultSetDescriptor
type ResultSetDescriptor interface {
	GUIDs() ([]string, error)
	Sort(column string, desc bool) error
}

type Client struct {
	restClient         restclient.Interface
	spaceFilteringOpts *authorization.SpaceFilteringOpts
}

func NewClient(restClient restclient.Interface, spaceFilteringOpts *authorization.SpaceFilteringOpts) *Client {
	return &Client{
		restClient:         restClient,
		spaceFilteringOpts: spaceFilteringOpts,
	}
}

func (c *Client) List(ctx context.Context, listObjectGVK schema.GroupVersionKind, opts ...client.ListOption) (ResultSetDescriptor, error) {
	listOpts, err := c.spaceFilteringOpts.Apply(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to apply space filtering options: %w", err)
	}

	pluralObjectKind := fmt.Sprintf("%ss", strings.ToLower(strings.TrimSuffix(listObjectGVK.Kind, "List")))
	requestPath := fmt.Sprintf("/apis/%s/%s/%s",
		listObjectGVK.Group,
		listObjectGVK.Version,
		pluralObjectKind,
	)

	table := &metav1.Table{}
	err = c.restClient.Get().
		AbsPath(requestPath).
		SetHeader("Accept", "application/json;as=Table;g=meta.k8s.io;v=v1").
		VersionedParams(&metav1.TableOptions{
			IncludeObject: metav1.IncludeNone,
		}, scheme.ParameterCodec).
		VersionedParams(listOpts.AsListOptions(), scheme.ParameterCodec).
		Do(ctx).Into(table)
	if err != nil {
		return nil, fmt.Errorf("failed to list %s/%s/%s descriptors: %w", listObjectGVK.Group, listObjectGVK.Version, pluralObjectKind, err)
	}

	return &TableResultSetDescriptor{Table: table}, nil
}
