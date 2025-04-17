package repositories

import (
	"context"

	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/watch"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

//counterfeiter:generate -o fake -fake-name Klient . Klient
type Klient interface {
	Get(ctx context.Context, obj client.Object) error
	Create(ctx context.Context, obj client.Object) error
	Patch(ctx context.Context, obj client.Object, modify func() error) error
	List(ctx context.Context, list client.ObjectList, opts ...ListOption) error
	Watch(ctx context.Context, obj client.ObjectList, opts ...ListOption) (watch.Interface, error)
	Delete(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error
}

type ListOptions struct {
	SpaceGUIDs    []string
	LabelSelector labels.Selector
	Namespace     string
	FieldSelector fields.Selector
}

type ListOption interface {
	ApplyToList(*ListOptions)
}

type WithSpaceGUIDs []string

func (o WithSpaceGUIDs) ApplyToList(opts *ListOptions) {
	opts.SpaceGUIDs = []string(o)
}

type InNamespace string

func (o InNamespace) ApplyToList(opts *ListOptions) {
	opts.Namespace = string(o)
}

type WithLabels struct {
	labels.Selector
}

func (o WithLabels) ApplyToList(opts *ListOptions) {
	opts.LabelSelector = labels.Selector(o)
}

type MatchingFields fields.Set

func (m MatchingFields) ApplyToList(opts *ListOptions) {
	sel := fields.Set(m).AsSelector()
	opts.FieldSelector = sel
}
