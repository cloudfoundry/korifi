package authorization

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

func NewSpaceFilteringClient(
	client client.WithWatch,
	privilegedClient client.WithWatch,
	spaceFilteringOpts *SpaceFilteringOpts,
) SpaceFilteringClient {
	return SpaceFilteringClient{
		WithWatch:          client,
		privilegedClient:   privilegedClient,
		spaceFilteringOpts: spaceFilteringOpts,
	}
}

type SpaceFilteringClient struct {
	client.WithWatch
	privilegedClient   client.WithWatch
	spaceFilteringOpts *SpaceFilteringOpts
}

func (c SpaceFilteringClient) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	effectiveListOpts, err := c.spaceFilteringOpts.Apply(ctx, opts...)
	if err != nil {
		return err
	}

	if effectiveListOpts.Namespace != "" {
		return c.WithWatch.List(ctx, list, effectiveListOpts)
	}

	return c.privilegedClient.List(ctx, list, effectiveListOpts)
}
