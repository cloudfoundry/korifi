package repositories

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
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
	Namespace     string
	FieldSelector fields.Selector
	Requrements   []labels.Requirement
}

type ListOption interface {
	ApplyToList(*ListOptions) error
}

type LabelOpt struct {
	Key   string
	Value string
}

func WithLabel(key, value string) ListOption {
	return LabelOpt{
		Key:   key,
		Value: value,
	}
}

func (o LabelOpt) ApplyToList(opts *ListOptions) error {
	req, err := labels.NewRequirement(o.Key, selection.Equals, []string{o.Value})
	if err != nil {
		return fmt.Errorf("invalid label selector: %w", err)
	}

	opts.Requrements = append(opts.Requrements, *req)
	return nil
}

type LabelIn struct {
	Key    string
	Values []string
}

func WithLabelIn(key string, values []string) ListOption {
	return LabelIn{
		Key:    key,
		Values: values,
	}
}

func (o LabelIn) ApplyToList(opts *ListOptions) error {
	if len(o.Values) == 0 {
		return nil
	}

	req, err := labels.NewRequirement(o.Key, selection.In, o.Values)
	if err != nil {
		return fmt.Errorf("invalid label selector: %w", err)
	}

	opts.Requrements = append(opts.Requrements, *req)
	return nil
}

type WithLabelSelector string

func (o WithLabelSelector) ApplyToList(opts *ListOptions) error {
	selector, err := labels.Parse(string(o))
	if err != nil {
		return fmt.Errorf("invalid label selector: %w", err)
	}
	reqs, _ := selector.Requirements()

	opts.Requrements = append(opts.Requrements, reqs...)
	return nil
}

type InNamespace string

func (o InNamespace) ApplyToList(opts *ListOptions) error {
	opts.Namespace = string(o)
	return nil
}

type MatchingFields fields.Set

func (m MatchingFields) ApplyToList(opts *ListOptions) error {
	sel := fields.Set(m).AsSelector()
	opts.FieldSelector = sel
	return nil
}
