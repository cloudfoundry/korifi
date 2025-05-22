package descriptors

import (
	"context"
	"fmt"
	"reflect"

	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type ObjectListMapper struct {
	client client.Client
}

func NewObjectListMapper(client client.Client) *ObjectListMapper {
	return &ObjectListMapper{
		client: client,
	}
}

func (m *ObjectListMapper) GUIDsToObjectList(ctx context.Context, gvk schema.GroupVersionKind, orderedGUIDs []string) (client.ObjectList, error) {
	listObj, err := scheme.Scheme.New(gvk)
	if err != nil {
		return nil, fmt.Errorf("failed to create new object list: %w", err)
	}

	list, ok := listObj.(client.ObjectList)
	if !ok {
		return nil, fmt.Errorf("object list is not a client.ObjectList: %T", listObj)
	}

	req, err := labels.NewRequirement("korifi.cloudfoundry.org/guid", selection.In, orderedGUIDs)
	if err != nil {
		return nil, fmt.Errorf("invalid label selector: %w", err)
	}
	err = m.client.List(ctx, list, client.MatchingLabelsSelector{
		Selector: labels.NewSelector().Add(*req),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list objects: %w", err)
	}

	return order(orderedGUIDs, gvk, list)
}

func order(orderedGUIDs []string, gvk schema.GroupVersionKind, list client.ObjectList) (client.ObjectList, error) {
	resultList, err := scheme.Scheme.New(gvk)
	if err != nil {
		return nil, fmt.Errorf("failed to create new object list: %w", err)
	}

	itemsIndex, err := getItemsIndex(list)
	if err != nil {
		return nil, fmt.Errorf("failed to get items index: %w", err)
	}

	for _, guid := range orderedGUIDs {
		item, ok := itemsIndex[guid]
		if !ok {
			return nil, fmt.Errorf("item with GUID %s not found in list", guid)
		}
		if err = appendToList(resultList, item); err != nil {
			return nil, fmt.Errorf("failed to append item to list: %w", err)
		}
	}

	return resultList.(client.ObjectList), nil
}

func getItemsIndex(listObj runtime.Object) (map[string]client.Object, error) {
	result := map[string]client.Object{}

	v := reflect.ValueOf(listObj)
	if v.Kind() != reflect.Ptr || v.IsNil() {
		return nil, fmt.Errorf("listObj must be a non-nil pointer")
	}

	// Dereference the list struct
	elem := v.Elem()
	itemsField := elem.FieldByName("Items")

	if !itemsField.IsValid() || itemsField.Kind() != reflect.Slice {
		return nil, fmt.Errorf("list object has no slice field named 'Items'")
	}

	for i := 0; i < itemsField.Len(); i++ {
		item := itemsField.Index(i).Addr().Interface()
		if obj, ok := item.(client.Object); ok {
			result[obj.GetName()] = obj
		} else {
			return nil, fmt.Errorf("item is not a client.Object: %T", item)
		}
	}

	return result, nil
}

func appendToList(listObj runtime.Object, item client.Object) error {
	v := reflect.ValueOf(listObj)
	if v.Kind() != reflect.Ptr || v.IsNil() {
		return fmt.Errorf("listObj must be a non-nil pointer")
	}

	// Dereference the list struct
	elem := v.Elem()
	itemsField := elem.FieldByName("Items")

	if !itemsField.IsValid() || itemsField.Kind() != reflect.Slice {
		return fmt.Errorf("list object has no slice field named 'Items'")
	}

	// Get the item value (concrete, not pointer)
	itemVal := reflect.ValueOf(item).Elem()

	// Append item to list
	itemsField.Set(reflect.Append(itemsField, itemVal))
	return nil
}
