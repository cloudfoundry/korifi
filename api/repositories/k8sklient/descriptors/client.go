package descriptors

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"code.cloudfoundry.org/korifi/api/repositories/k8sklient"
	"github.com/BooleanCat/go-functional/v2/it"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/scheme"
	restclient "k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Client struct {
	restClient restclient.Interface
}

func NewClient(restClient restclient.Interface) *Client {
	return &Client{
		restClient: restClient,
	}
}

func (c *Client) List(ctx context.Context, listObjectGVK schema.GroupVersionKind, opts ...client.ListOption) (k8sklient.ResultSetDescriptor, error) {
	listOpts := unpackListOptions(opts...)

	pluralObjectKind := fmt.Sprintf("%ss", strings.ToLower(strings.TrimSuffix(listObjectGVK.Kind, "List")))
	requestPath := fmt.Sprintf("/apis/%s/%s/%s",
		listObjectGVK.Group,
		listObjectGVK.Version,
		pluralObjectKind,
	)

	table := &metav1.Table{}
	err := c.restClient.Get().
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

	return &tableResultSetDescriptor{table: table}, nil
}

func unpackListOptions(opts ...client.ListOption) client.ListOptions {
	listOpts := client.ListOptions{}
	for _, o := range opts {
		o.ApplyToList(&listOpts)
	}
	return listOpts
}

type tableResultSetDescriptor struct {
	table *metav1.Table
}

func (d *tableResultSetDescriptor) SortedGUIDs(column string, desc bool) ([]string, error) {
	sortColumnIndex, sortColumn, err := d.getColumn(column)
	if err != nil {
		return nil, err
	}

	nameColumnIndex, nameColumnDef, err := d.getColumn("Name")
	if err != nil {
		return nil, err
	}

	if nameColumnDef.Type != "string" {
		return nil, fmt.Errorf("column 'name' is not of type string")
	}

	var pairs PairsList = slices.Collect(it.Map(slices.Values(d.table.Rows), func(row metav1.TableRow) *Pair {
		return &Pair{
			Guid:      row.Cells[nameColumnIndex].(string),
			Value:     row.Cells[sortColumnIndex],
			ValueType: sortColumn.Type,
		}
	}))

	pairs.Sort(desc)

	return slices.Collect(it.Map(slices.Values(pairs), func(pair *Pair) string {
		return pair.Guid
	})), nil
}

func (d *tableResultSetDescriptor) getColumn(column string) (int, metav1.TableColumnDefinition, error) {
	for i, columnDef := range d.table.ColumnDefinitions {
		if columnDef.Name == column {
			return i, columnDef, nil
		}
	}

	return -1, metav1.TableColumnDefinition{}, fmt.Errorf("column %s not found", column)
}
