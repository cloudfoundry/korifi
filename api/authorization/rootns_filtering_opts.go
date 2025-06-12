package authorization

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

type RootNsFilteringOpts struct {
	rootNamespace string
}

func NewRootNsFilteringOpts(rootNamespace string) *RootNsFilteringOpts {
	return &RootNsFilteringOpts{
		rootNamespace: rootNamespace,
	}
}

func (o *RootNsFilteringOpts) Apply(ctx context.Context, opts ...client.ListOption) (*client.ListOptions, error) {
	effectiveListOpts := &client.ListOptions{}
	for _, o := range opts {
		o.ApplyToList(effectiveListOpts)
	}

	effectiveListOpts.Namespace = o.rootNamespace

	return effectiveListOpts, nil
}
