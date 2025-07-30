package descriptors

import (
	"context"
	"fmt"
	"reflect"

	"code.cloudfoundry.org/korifi/api/authorization"
	"code.cloudfoundry.org/korifi/api/repositories/k8sklient/descriptors/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type ObjectListMapper struct {
	userClientFactory authorization.UserClientFactory
}

func NewObjectListMapper(userClientFactory authorization.UserClientFactory) *ObjectListMapper {
	return &ObjectListMapper{
		userClientFactory: userClientFactory,
	}
}

func (m *ObjectListMapper) GUIDsToObjectList(ctx context.Context, listObjectGVK schema.GroupVersionKind, orderedGUIDs []string) (client.ObjectList, error) {
	listObj, err := scheme.Scheme.New(listObjectGVK)
	if err != nil {
		return nil, fmt.Errorf("failed to create new object list: %w", err)
	}

	list, ok := listObj.(client.ObjectList)
	if !ok {
		return nil, fmt.Errorf("object list is not a client.ObjectList: %T", listObj)
	}

	if len(orderedGUIDs) == 0 {
		return list, nil
	}

	req, err := labels.NewRequirement("korifi.cloudfoundry.org/guid", selection.In, orderedGUIDs)
	if err != nil {
		return nil, fmt.Errorf("invalid label selector: %w", err)
	}

	authInfo, _ := authorization.InfoFromContext(ctx)
	userClient, err := m.userClientFactory.BuildClient(authInfo)
	if err != nil {
		return nil, fmt.Errorf("failed to build user client: %w", err)
	}

	err = userClient.List(ctx, list, client.MatchingLabelsSelector{
		Selector: labels.NewSelector().Add(*req),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list objects: %w", err)
	}

	return order(orderedGUIDs, listObjectGVK, list)
}

func order(orderedGUIDs []string, listObjectGVK schema.GroupVersionKind, list client.ObjectList) (client.ObjectList, error) {
	resultList, err := scheme.Scheme.New(listObjectGVK)
	if err != nil {
		return nil, fmt.Errorf("failed to create new object list: %w", err)
	}

	itemsIndex, err := getItemsIndex(list)
	if err != nil {
		return nil, fmt.Errorf("failed to get items index: %w", err)
	}

	orderedObjects := []client.Object{}
	for _, guid := range orderedGUIDs {
		item, ok := itemsIndex[guid]
		if !ok {
			return nil, errors.NewObjectResolutionError(guid, listObjectGVK)
		}
		orderedObjects = append(orderedObjects, item)
	}

	if err = appendToList(resultList, orderedObjects); err != nil {
		return nil, fmt.Errorf("failed to append item to list: %w", err)
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

	for i := range itemsField.Len() {
		item := itemsField.Index(i).Addr().Interface()
		if obj, ok := item.(client.Object); ok {
			result[obj.GetName()] = obj
		} else {
			return nil, fmt.Errorf("item is not a client.Object: %T", item)
		}
	}

	return result, nil
}

func appendToList(listObj runtime.Object, items []client.Object) error {
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

	for _, item := range items {
		// Get the item value (concrete, not pointer)
		itemVal := reflect.ValueOf(item).Elem()
		itemsField.Set(reflect.Append(itemsField, itemVal))
	}

	return nil
}
