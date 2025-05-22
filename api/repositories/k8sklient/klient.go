package k8sklient

import (
	"context"
	"fmt"
	"net/http"

	"code.cloudfoundry.org/korifi/api/authorization"
	"code.cloudfoundry.org/korifi/api/repositories"
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes/scheme"
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
}

//counterfeiter:generate -o fake -fake-name DescriptorClient . DescriptorClient
type DescriptorClient interface {
	List(ctx context.Context, gvk schema.GroupVersionKind, opts ...client.ListOption) (ResultSetDescriptor, error)
}

//counterfeiter:generate -o fake -fake-name ResultSetDescriptor . ResultSetDescriptor
type ResultSetDescriptor interface {
	SortedGUIDs(column string, desc bool) ([]string, error)
}

//counterfeiter:generate -o fake -fake-name ObjectListMapper . ObjectListMapper
type ObjectListMapper interface {
	GUIDsToObjectList(ctx context.Context, gvk schema.GroupVersionKind, orderedGUIDs []string) (client.ObjectList, error)
}

func NewK8sKlient(
	namespaceRetriever NamespaceRetriever,
	descriptorClient DescriptorClient,
	objectListMapper ObjectListMapper,
	userClientFactory authorization.UserClientFactory,
) *K8sKlient {
	return &K8sKlient{
		namespaceRetriever: namespaceRetriever,
		descriptorClient:   descriptorClient,
		objectListMapper:   objectListMapper,
		userClientFactory:  userClientFactory,
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

func (k *K8sKlient) List(ctx context.Context, list client.ObjectList, opts ...repositories.ListOption) error {
	listOpts, err := unpackListOptions(opts...)
	if err != nil {
		return toStatusError(list, err)
	}

	authInfo, _ := authorization.InfoFromContext(ctx)
	userClient, err := k.userClientFactory.BuildClient(authInfo)
	if err != nil {
		return fmt.Errorf("failed to build user client: %w", err)
	}

	if listOpts.Sort != nil {
		// TODO: move to descriptors/client.go
		//
		// table := &metav1.Table{}
		// if err := k.restClient.
		// 	Get().AbsPath("/apis/korifi.cloudfoundry.org/v1alpha1/cfapps").
		// 	SetHeader("Accept", "application/json;as=Table;g=meta.k8s.io;v=v1").
		// 	VersionedParams(&metav1.TableOptions{
		// 		IncludeObject: metav1.IncludeNone,
		// 	}, scheme.ParameterCodec).
		// 	VersionedParams(listOpts.AsClientListOptions().AsListOptions(), scheme.ParameterCodec).
		// 	Do(ctx).Into(table); err != nil {
		// 	return fmt.Errorf("failed to list app descriptors: %w", err)
		// }

		gvk, err := getGVK(list)
		objectDescriptors, err := k.descriptorClient.List(ctx, gvk, listOpts.AsClientListOptions())
		if err != nil {
			return fmt.Errorf("failed to list object descriptors: %w", err)
		}

		sortedObjectGUIDs, err := objectDescriptors.SortedGUIDs(listOpts.Sort.By, listOpts.Sort.Desc)
		if err != nil {
			return fmt.Errorf("failed to get sorted object GUIDs: %w", err)
		}

		if err != nil {
			return fmt.Errorf("failed to get GVK: %w", err)
		}
		list, err = k.objectListMapper.GUIDsToObjectList(ctx, gvk, sortedObjectGUIDs)
		if err != nil {
			return fmt.Errorf("failed to map sorted object GUIDs to objects: %w", err)
		}
		return nil

	}

	return userClient.List(ctx, list, listOpts.AsClientListOptions())
}

func getGVK(obj runtime.Object) (schema.GroupVersionKind, error) {
	gvks, _, err := scheme.Scheme.ObjectKinds(obj)
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
