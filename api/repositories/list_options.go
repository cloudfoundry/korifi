package repositories

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	"code.cloudfoundry.org/korifi/api/repositories/k8sklient/descriptors"
	"code.cloudfoundry.org/korifi/tools/k8s"
	"github.com/BooleanCat/go-functional/v2/it"
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
	List(ctx context.Context, list client.ObjectList, opts ...ListOption) (descriptors.PageInfo, error)
	Watch(ctx context.Context, obj client.ObjectList, opts ...ListOption) (watch.Interface, error)
	Delete(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error
}

type ListOptions struct {
	Namespace     string
	FieldSelector fields.Selector
	Requrements   []labels.Requirement
	Sort          *SortOpt
	Paging        *PagingOpt
}

func (o ListOptions) AsClientListOptions() *client.ListOptions {
	return &client.ListOptions{
		LabelSelector: newLabelSelector(o.Requrements),
		FieldSelector: o.FieldSelector,
		Namespace:     o.Namespace,
	}
}

func newLabelSelector(requrements []labels.Requirement) labels.Selector {
	if len(requrements) == 0 {
		return nil
	}

	return labels.NewSelector().Add(requrements...)
}

type ListOption interface {
	ApplyToList(*ListOptions) error
}

type NoopListOption struct{}

func (o NoopListOption) ApplyToList(opts *ListOptions) error {
	return nil
}

type ErroringListOption string

func (o ErroringListOption) ApplyToList(opts *ListOptions) error {
	return errors.New(string(o))
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

type LabelExists struct {
	Key string
}

func WithLabelExists(key string) ListOption {
	return LabelExists{
		Key: key,
	}
}

func (o LabelExists) ApplyToList(opts *ListOptions) error {
	req, err := labels.NewRequirement(o.Key, selection.Exists, nil)
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

type Nothing struct{}

func (o Nothing) ApplyToList(opts *ListOptions) error {
	matchNothingRequirements, _ := k8s.MatchNotingSelector().Requirements()
	opts.Requrements = append(opts.Requrements, matchNothingRequirements...)

	return nil
}

func WithLabelStrictlyIn(key string, values []string) ListOption {
	if len(values) == 0 {
		return Nothing{}
	}

	return WithLabelIn(key, values)
}

func WithOrdering(orderBy string, orderByToColumnMappings ...string) ListOption {
	orderByToColumn, err := asMap(orderByToColumnMappings...)
	if err != nil {
		return ErroringListOption(fmt.Sprintf("invalid orderBy to column mappings: %s", err.Error()))
	}

	desc := false
	if strings.HasPrefix(orderBy, "-") {
		desc = true
		orderBy = strings.TrimPrefix(orderBy, "-")
	}

	if orderBy == "" {
		return NoopListOption{}
	}

	column, ok := orderByToColumn[orderBy]
	if !ok {
		return ErroringListOption(fmt.Sprintf("unsupported field for ordering: %q", orderBy))
	}

	return SortOpt{
		By:   column,
		Desc: desc,
	}
}

func asMap(pairs ...string) (map[string]string, error) {
	pairs = append(
		[]string{
			"created_at", "Created At",
			"updated_at", "Updated At",
		},
		pairs...,
	)

	if len(pairs)%2 != 0 {
		return nil, fmt.Errorf("expected even number of key value pairs, got %d elements", len(pairs))
	}

	indexedPairsIter := it.Enumerate(slices.Values(pairs))
	keysIter := it.Right(it.Filter2(indexedPairsIter, func(i int, _ string) bool {
		return i%2 == 0
	}))
	valuesIter := it.Right(it.Filter2(indexedPairsIter, func(i int, _ string) bool {
		return i%2 == 1
	}))

	return maps.Collect(it.Zip(keysIter, valuesIter)), nil
}

type SortOpt struct {
	By   string
	Desc bool
}

func (o SortOpt) ApplyToList(opts *ListOptions) error {
	opts.Sort = &o
	return nil
}

func WithPaging(pagination Pagination) ListOption {
	if pagination.PerPage == 0 || pagination.Page == 0 {
		return NoopListOption{}
	}

	return PagingOpt{
		PageSize:   pagination.PerPage,
		PageNumber: pagination.Page,
	}
}

type PagingOpt struct {
	PageSize   int
	PageNumber int
}

func (o PagingOpt) ApplyToList(opts *ListOptions) error {
	opts.Paging = &o
	return nil
}
