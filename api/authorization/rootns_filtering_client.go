package authorization

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

func NewRootNSFilteringClient(
	client client.WithWatch,
	rootNS string,
) RootNSFilteringClient {
	return RootNSFilteringClient{
		WithWatch: client,
		rootNS:    rootNS,
	}
}

type RootNSFilteringClient struct {
	client.WithWatch
	rootNS string
}

func (c RootNSFilteringClient) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	effectiveListOpts := append(opts, client.InNamespace(c.rootNS))
	return c.WithWatch.List(ctx, list, effectiveListOpts...)
}
