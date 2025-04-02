package k8sklient

import (
	"context"
	"fmt"

	"code.cloudfoundry.org/korifi/api/authorization"
	"code.cloudfoundry.org/korifi/api/repositories"
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/apimachinery/pkg/watch"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

//counterfeiter:generate -o fake -fake-name NamespaceRetriever . NamespaceRetriever
type NamespaceRetriever interface {
	NamespaceFor(ctx context.Context, resourceGUID, resourceType string) (string, error)
}

type K8sKlient struct {
	namespaceRetriever NamespaceRetriever
	userClientFactory  authorization.UserClientFactory
}

func NewK8sKlient(
	namespaceRetriever NamespaceRetriever,
	userClientFactory authorization.UserClientFactory,
) *K8sKlient {
	return &K8sKlient{
		namespaceRetriever: namespaceRetriever,
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
	authInfo, _ := authorization.InfoFromContext(ctx)
	userClient, err := k.userClientFactory.BuildClient(authInfo)
	if err != nil {
		return fmt.Errorf("failed to build user client: %w", err)
	}

	k8sListOpts, err := toK8sListOptions(opts...)
	if err != nil {
		return err
	}

	return userClient.List(ctx, list, &k8sListOpts)
}

func (k *K8sKlient) Watch(ctx context.Context, obj client.ObjectList, opts ...repositories.ListOption) (watch.Interface, error) {
	authInfo, _ := authorization.InfoFromContext(ctx)
	userClient, err := k.userClientFactory.BuildClient(authInfo)
	if err != nil {
		return nil, fmt.Errorf("failed to build user client: %w", err)
	}

	k8sListOpts, err := toK8sListOptions(opts...)
	if err != nil {
		return nil, err
	}

	return userClient.Watch(ctx, obj, &k8sListOpts)
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

func toK8sListOptions(opts ...repositories.ListOption) (client.ListOptions, error) {
	if len(opts) == 0 {
		return client.ListOptions{}, nil
	}

	listOpts := repositories.ListOptions{}
	for _, o := range opts {
		o.ApplyToList(&listOpts)
	}

	k8sListOptions := client.ListOptions{
		FieldSelector: listOpts.FieldSelector,
	}

	if listOpts.Namespace != "" {
		k8sListOptions.Namespace = listOpts.Namespace
	}

	labelSelector, err := toLabelSelector(listOpts)
	if err != nil {
		return client.ListOptions{}, err
	}
	k8sListOptions.LabelSelector = labelSelector

	return k8sListOptions, nil
}

func toLabelSelector(listOpts repositories.ListOptions) (labels.Selector, error) {
	labelSelectorReqs := labels.Requirements{}
	if listOpts.LabelSelector != nil {
		reqs, _ := listOpts.LabelSelector.Requirements()
		labelSelectorReqs = append(labelSelectorReqs, reqs...)
	}

	if len(listOpts.SpaceGUIDs) > 0 {
		spaceGuidsReq, err := labels.NewRequirement(korifiv1alpha1.SpaceGUIDKey, selection.In, listOpts.SpaceGUIDs)
		if err != nil {
			return nil, fmt.Errorf("failed to create space guids label selector: %w", err)
		}

		labelSelectorReqs = append(labelSelectorReqs, *spaceGuidsReq)
	}

	if len(labelSelectorReqs) == 0 {
		return nil, nil
	}

	return labels.NewSelector().Add(labelSelectorReqs...), nil
}
