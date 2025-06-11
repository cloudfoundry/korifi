package k8sklient

import (
	"context"
	"fmt"
	"net/http"
	"reflect"

	"code.cloudfoundry.org/korifi/api/authorization"
	"code.cloudfoundry.org/korifi/api/repositories"
	"code.cloudfoundry.org/korifi/api/repositories/k8sklient/descriptors"
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

//counterfeiter:generate -o fake -fake-name NamespaceRetriever . NamespaceRetriever
type NamespaceRetriever interface {
	NamespaceFor(ctx context.Context, resourceGUID, resourceType string) (string, error)
}

type K8sKlient struct {
	namespaceRetriever NamespaceRetriever
	descriptorClient   DescriptorClient
	objectListMapper   ObjectListMapper
	userClientFactory  authorization.UserClientFactory
	scheme             *runtime.Scheme
}

//counterfeiter:generate -o fake -fake-name DescriptorClient . DescriptorClient
type DescriptorClient interface {
	List(ctx context.Context, listObjectGVK schema.GroupVersionKind, opts ...client.ListOption) (descriptors.ResultSetDescriptor, error)
}

//counterfeiter:generate -o fake -fake-name ObjectListMapper . ObjectListMapper
type ObjectListMapper interface {
	GUIDsToObjectList(ctx context.Context, listObjectGVK schema.GroupVersionKind, orderedGUIDs []string) (client.ObjectList, error)
}

func NewK8sKlient(
	namespaceRetriever NamespaceRetriever,
	descriptorClient DescriptorClient,
	objectListMapper ObjectListMapper,
	userClientFactory authorization.UserClientFactory,
	scheme *runtime.Scheme,
) *K8sKlient {
	return &K8sKlient{
		namespaceRetriever: namespaceRetriever,
		descriptorClient:   descriptorClient,
		objectListMapper:   objectListMapper,
		userClientFactory:  userClientFactory,
		scheme:             scheme,
	}
}

func (k *K8sKlient) Get(ctx context.Context, obj client.Object) error {
	authInfo, _ := authorization.InfoFromContext(ctx)

	guid := obj.GetName()
	ns, err := k.resolveNamespace(ctx, obj)
	if err != nil {
		return err
	}

	userClient, err := k.userClientFactory.BuildClient(authInfo)
	if err != nil {
		return fmt.Errorf("failed to build user client: %w", err)
	}

	return userClient.Get(ctx, client.ObjectKey{Namespace: ns, Name: guid}, obj)
}

func (k *K8sKlient) Create(ctx context.Context, obj client.Object) error {
	authInfo, _ := authorization.InfoFromContext(ctx)
	userClient, err := k.userClientFactory.BuildClient(authInfo)
	if err != nil {
		return fmt.Errorf("failed to build user client: %w", err)
	}

	return userClient.Create(ctx, obj)
}

func (k *K8sKlient) Patch(ctx context.Context, obj client.Object, modify func() error) error {
	authInfo, _ := authorization.InfoFromContext(ctx)
	userClient, err := k.userClientFactory.BuildClient(authInfo)
	if err != nil {
		return fmt.Errorf("failed to build user client: %w", err)
	}

	oldObject := obj.DeepCopyObject().(client.Object)

	err = modify()
	if err != nil {
		return err
	}

	err = userClient.Patch(ctx, obj, client.MergeFrom(oldObject))
	if err != nil {
		return fmt.Errorf("failed to patch: %w", err)
	}

	return nil
}

func (k *K8sKlient) List(ctx context.Context, list client.ObjectList, opts ...repositories.ListOption) (descriptors.PageInfo, error) {
	listOpts, err := unpackListOptions(opts...)
	if err != nil {
		return descriptors.PageInfo{}, toStatusError(list, err)
	}

	if isSimpleList(listOpts) {
		return k.listViaUserClient(ctx, list, listOpts.AsClientListOptions())
	}

	listObjectGVK, err := k.getGVK(list)
	if err != nil {
		return descriptors.PageInfo{}, fmt.Errorf("failed to get GVK for list %T: %w", list, err)
	}

	objectGUIDs, err := k.fetchObjectGUIDs(ctx, listObjectGVK, listOpts)
	if err != nil {
		return descriptors.PageInfo{}, fmt.Errorf("failed to fetch object guids: %w", err)
	}

	var pageInfo descriptors.PageInfo
	objectGUIDs, pageInfo, err = pageGUIDs(objectGUIDs, listOpts)
	if err != nil {
		return descriptors.PageInfo{}, fmt.Errorf("failed to page object guids: %w", err)
	}

	listResult, err := k.objectListMapper.GUIDsToObjectList(ctx, listObjectGVK, objectGUIDs)
	if err != nil {
		return descriptors.PageInfo{}, fmt.Errorf("failed to map sorted object GUIDs to objects: %w", err)
	}

	if err := transferItems(listResult, list); err != nil {
		return descriptors.PageInfo{}, fmt.Errorf("failed to copy list items: %w", err)
	}

	return pageInfo, nil
}

func (k *K8sKlient) fetchObjectGUIDs(ctx context.Context, listObjectGVK schema.GroupVersionKind, listOpts repositories.ListOptions) ([]string, error) {
	objectDescriptors, err := k.descriptorClient.List(ctx, listObjectGVK, listOpts.AsClientListOptions())
	if err != nil {
		return nil, fmt.Errorf("failed to list object descriptors: %w", err)
	}

	if listOpts.Sort != nil {
		if err = objectDescriptors.Sort(listOpts.Sort.By, listOpts.Sort.Desc); err != nil {
			return nil, fmt.Errorf("failed to sort object descriptors: %w", err)
		}
	}

	objectGUIDs, err := objectDescriptors.GUIDs()
	if err != nil {
		return nil, fmt.Errorf("failed to get sorted object GUIDs: %w", err)
	}

	return objectGUIDs, nil
}

func pageGUIDs(objectGUIDs []string, listOpts repositories.ListOptions) ([]string, descriptors.PageInfo, error) {
	if listOpts.Paging == nil {
		return objectGUIDs, descriptors.SinglePageInfo(len(objectGUIDs), len(objectGUIDs)), nil
	}

	page, err := descriptors.GetPage(objectGUIDs, listOpts.Paging.PageSize, listOpts.Paging.PageNumber)
	if err != nil {
		return nil, descriptors.PageInfo{}, fmt.Errorf("failed to page object guids: %w", err)
	}

	return page.Items, page.PageInfo, nil
}

func isSimpleList(listOpts repositories.ListOptions) bool {
	return listOpts.Sort == nil && listOpts.Paging == nil
}

func (k *K8sKlient) listViaUserClient(ctx context.Context, list client.ObjectList, opts ...client.ListOption) (descriptors.PageInfo, error) {
	authInfo, _ := authorization.InfoFromContext(ctx)
	userClient, err := k.userClientFactory.BuildClient(authInfo)
	if err != nil {
		return descriptors.PageInfo{}, fmt.Errorf("failed to build user client: %w", err)
	}

	err = userClient.List(ctx, list, opts...)
	if err != nil {
		return descriptors.PageInfo{}, err
	}

	itemsField, err := getObjectListItemsField(list)
	if err != nil {
		return descriptors.PageInfo{}, err
	}

	return descriptors.SinglePageInfo(itemsField.Len(), itemsField.Len()), nil
}

func getObjectListItemsField(listObj client.ObjectList) (reflect.Value, error) {
	v := reflect.ValueOf(listObj)
	if v.Kind() != reflect.Ptr || v.IsNil() {
		return reflect.Value{}, fmt.Errorf("listObj must be a non-nil pointer")
	}

	// Dereference the list struct
	elem := v.Elem()
	itemsField := elem.FieldByName("Items")

	if !itemsField.IsValid() || itemsField.Kind() != reflect.Slice {
		return reflect.Value{}, fmt.Errorf("list object has no slice field named 'Items'")
	}

	return itemsField, nil
}

func transferItems(srcList, destList client.ObjectList) error {
	destItemsField, err := getObjectListItemsField(destList)
	if err != nil {
		return fmt.Errorf("failed to get items field from destination list: %w", err)
	}

	srcItemsField, err := getObjectListItemsField(srcList)
	if err != nil {
		return fmt.Errorf("failed to get items field from source list: %w", err)
	}
	destItemsField.Set(srcItemsField)

	return nil
}

func (k *K8sKlient) getGVK(obj runtime.Object) (schema.GroupVersionKind, error) {
	gvks, _, err := k.scheme.ObjectKinds(obj)
	if err != nil {
		return schema.GroupVersionKind{}, err
	}
	if len(gvks) == 0 {
		return schema.GroupVersionKind{}, fmt.Errorf("no GVK found for object")
	}
	return gvks[0], nil
}

func unpackListOptions(opts ...repositories.ListOption) (repositories.ListOptions, error) {
	listOpts := repositories.ListOptions{}
	for _, o := range opts {
		if err := o.ApplyToList(&listOpts); err != nil {
			return repositories.ListOptions{}, err
		}
	}
	return listOpts, nil
}

func toStatusError(list client.ObjectList, err error) *k8serrors.StatusError {
	return &k8serrors.StatusError{
		ErrStatus: metav1.Status{
			Message: err.Error(),
			Status:  metav1.StatusFailure,
			Code:    http.StatusUnprocessableEntity,
			Reason:  metav1.StatusReasonInvalid,
		},
	}
}

func (k *K8sKlient) Watch(ctx context.Context, list client.ObjectList, opts ...repositories.ListOption) (watch.Interface, error) {
	listOpts, err := unpackListOptions(opts...)
	if err != nil {
		return nil, toStatusError(list, err)
	}

	authInfo, _ := authorization.InfoFromContext(ctx)
	userClient, err := k.userClientFactory.BuildClient(authInfo)
	if err != nil {
		return nil, fmt.Errorf("failed to build user client: %w", err)
	}

	return userClient.Watch(ctx, list, listOpts.AsClientListOptions())
}

func (k *K8sKlient) Delete(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
	authInfo, _ := authorization.InfoFromContext(ctx)
	userClient, err := k.userClientFactory.BuildClient(authInfo)
	if err != nil {
		return fmt.Errorf("failed to build user client: %w", err)
	}
	return userClient.Delete(ctx, obj, opts...)
}

func (k *K8sKlient) resolveNamespace(ctx context.Context, obj client.Object) (string, error) {
	if obj.GetNamespace() != "" {
		return obj.GetNamespace(), nil
	}

	resourceType, err := getResourceType(obj)
	if err != nil {
		return "", err
	}

	return k.namespaceRetriever.NamespaceFor(ctx, obj.GetName(), resourceType)
}

func getResourceType(obj client.Object) (string, error) {
	switch obj.(type) {
	case *korifiv1alpha1.CFApp:
		return repositories.AppResourceType, nil
	case *korifiv1alpha1.CFBuild:
		return repositories.BuildResourceType, nil
	case *korifiv1alpha1.CFDomain:
		return repositories.DomainResourceType, nil
	case *korifiv1alpha1.CFPackage:
		return repositories.PackageResourceType, nil
	case *korifiv1alpha1.CFProcess:
		return repositories.ProcessResourceType, nil
	case *korifiv1alpha1.CFSpace:
		return repositories.SpaceResourceType, nil
	case *korifiv1alpha1.CFRoute:
		return repositories.RouteResourceType, nil
	case *korifiv1alpha1.CFServiceBinding:
		return repositories.ServiceBindingResourceType, nil
	case *korifiv1alpha1.CFServiceInstance:
		return repositories.ServiceInstanceResourceType, nil
	case *korifiv1alpha1.CFTask:
		return repositories.TaskResourceType, nil
	default:
		return "", fmt.Errorf("unsupported resource type %T", obj)
	}
}
