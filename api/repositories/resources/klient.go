package resources

import (
	"context"
	"fmt"

	"code.cloudfoundry.org/korifi/api/authorization"
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/apimachinery/pkg/watch"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	AppResourceType = "App"
)

type ListOptions struct {
	guids         []string
	spaceGUIDs    []string
	labelSelector labels.Selector
	// TODO: add options for page, per_page, orderBy
	namespace     string
	fieldSelector fields.Selector
}

func (o ListOptions) ToK8sListOptions() (client.ListOptions, error) {
	labelSelector := labels.NewSelector()
	if o.labelSelector != nil {
		reqs, _ := o.labelSelector.Requirements()
		labelSelector = labelSelector.Add(reqs...)
	}

	if len(o.guids) > 0 {
		guidsReq, err := labels.NewRequirement(korifiv1alpha1.GUIDKey, selection.In, o.guids)
		if err != nil {
			return client.ListOptions{}, fmt.Errorf("failed to create guids label selector: %w", err)
		}

		labelSelector = labelSelector.Add(*guidsReq)
	}

	if len(o.spaceGUIDs) > 0 {
		spaceGuidsReq, err := labels.NewRequirement(korifiv1alpha1.SpaceGUIDKey, selection.In, o.spaceGUIDs)
		if err != nil {
			return client.ListOptions{}, fmt.Errorf("failed to create space guids label selector: %w", err)
		}

		labelSelector = labelSelector.Add(*spaceGuidsReq)
	}

	return client.ListOptions{
		LabelSelector: labelSelector,
		Namespace:     o.namespace,
		FieldSelector: o.fieldSelector,
	}, nil
}

type ListOption interface {
	ApplyToList(*ListOptions)
}

type WithGUIDs []string

func (o WithGUIDs) ApplyToList(opts *ListOptions) {
	opts.guids = []string(o)
}

type WithSpaceGUIDs []string

func (o WithSpaceGUIDs) ApplyToList(opts *ListOptions) {
	opts.spaceGUIDs = []string(o)
}

type WithLabels struct {
	labels.Selector
}

func (o WithLabels) ApplyToList(opts *ListOptions) {
	opts.labelSelector = labels.Selector(o)
}

type InNamespace string

func (n InNamespace) ApplyToList(opts *ListOptions) {
	opts.namespace = string(n)
}

type NamespaceRetriever interface {
	NamespaceFor(ctx context.Context, resourceGUID, resourceType string) (string, error)
}

type MatchingFields fields.Set

func (m MatchingFields) ApplyToList(opts *ListOptions) {
	sel := fields.Set(m).AsSelector()
	opts.fieldSelector = sel
}

type Klient interface {
	Get(ctx context.Context, obj client.Object) error
	Create(ctx context.Context, obj client.Object) error
	Patch(ctx context.Context, obj client.Object, modify func() error) error
	List(ctx context.Context, list client.ObjectList, opts ...ListOption) error
	Watch(ctx context.Context, obj client.ObjectList, opts ...ListOption) (watch.Interface, error)
	Delete(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error
}

var _ Klient = &K8sClientKlient{}

type K8sClientKlient struct {
	namespaceRetriever NamespaceRetriever
	userClientFactory  authorization.UserClientFactory
}

func NewKlient(
	namespaceRetriever NamespaceRetriever,
	userClientFactory authorization.UserClientFactory,
) *K8sClientKlient {
	return &K8sClientKlient{
		namespaceRetriever: namespaceRetriever,
		userClientFactory:  userClientFactory,
	}
}

func (k *K8sClientKlient) Get(ctx context.Context, obj client.Object) error {
	authInfo, _ := authorization.InfoFromContext(ctx)

	guid := obj.GetName()
	ns := obj.GetNamespace()

	if ns == "" {
		resourceType, err := getResourceType(obj)
		if err != nil {
			return err
		}

		ns, err = k.namespaceRetriever.NamespaceFor(ctx, guid, resourceType)
		if err != nil {
			return err
		}
	}

	userClient, err := k.userClientFactory.BuildClient(authInfo)
	if err != nil {
		return fmt.Errorf("failed to build user client: %w", err)
	}

	return userClient.Get(ctx, client.ObjectKey{Namespace: ns, Name: guid}, obj)
}

func (k *K8sClientKlient) Create(ctx context.Context, obj client.Object) error {
	authInfo, _ := authorization.InfoFromContext(ctx)
	userClient, err := k.userClientFactory.BuildClient(authInfo)
	if err != nil {
		return fmt.Errorf("failed to build user client: %w", err)
	}

	return userClient.Create(ctx, obj)
}

func (k *K8sClientKlient) Patch(ctx context.Context, obj client.Object, modify func() error) error {
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

func (k *K8sClientKlient) List(ctx context.Context, list client.ObjectList, opts ...ListOption) error {
	authInfo, _ := authorization.InfoFromContext(ctx)
	listOpts := ListOptions{}
	for _, o := range opts {
		o.ApplyToList(&listOpts)
	}

	k8sListOpts, err := listOpts.ToK8sListOptions()
	if err != nil {
		return err
	}

	userClient, err := k.userClientFactory.BuildClient(authInfo)
	return userClient.List(ctx, list, &k8sListOpts)
}

func (k *K8sClientKlient) Watch(ctx context.Context, obj client.ObjectList, opts ...ListOption) (watch.Interface, error) {
	authInfo, _ := authorization.InfoFromContext(ctx)
	userClient, err := k.userClientFactory.BuildClient(authInfo)
	if err != nil {
		return nil, fmt.Errorf("failed to build user client: %w", err)
	}

	listOpts := ListOptions{}
	for _, o := range opts {
		o.ApplyToList(&listOpts)
	}

	k8sListOpts, err := listOpts.ToK8sListOptions()
	if err != nil {
		return nil, err
	}

	return userClient.Watch(ctx, obj, &k8sListOpts)
}

func (k *K8sClientKlient) Delete(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
	authInfo, _ := authorization.InfoFromContext(ctx)
	userClient, err := k.userClientFactory.BuildClient(authInfo)
	if err != nil {
		return fmt.Errorf("failed to build user client: %w", err)
	}
	return userClient.Delete(ctx, obj, opts...)
}

func getResourceType(obj client.Object) (string, error) {
	switch obj.(type) {
	case *korifiv1alpha1.CFApp:
		return AppResourceType, nil
	default:
		return "", fmt.Errorf("unsupported resource type %T", obj)
	}
}
